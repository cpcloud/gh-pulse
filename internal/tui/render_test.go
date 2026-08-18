// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/gh-pulse/internal/history"
	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureSnapshot(t *testing.T) pulse.Snapshot {
	t.Helper()
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	archive, err := history.Calculate([]history.Interval{{Start: now.AddDate(0, 0, -2), End: now.AddDate(0, 0, -2).Add(time.Hour), Impact: history.ImpactMinor, Components: []string{"Git Operations", "API Requests"}}}, now)
	require.NoError(t, err)
	groupID := "core"
	return pulse.Snapshot{
		Overall: pulse.Overall{State: pulse.Operational, Description: "All Systems Operational", UpdatedAt: &now},
		Components: []pulse.Component{
			{ID: groupID, Name: "Core services", State: pulse.Operational, Group: true},
			{Name: "Git Operations", State: pulse.Operational, GroupID: &groupID},
			{Name: "API Requests", State: pulse.Minor},
			{Name: "Notifications", State: pulse.Operational},
		},
		RecentFeed: []pulse.FeedEntry{{Title: "Incident with API Requests", UpdatedAt: now}}, History: archive,
		Sources: pulse.Sources{Current: pulse.Source{Available: true, FetchedAt: &now}, Feed: pulse.Source{Available: true, UpdatedAt: &now}, History: pulse.Source{Available: true, FetchedAt: &now}},
	}
}

func TestRenderAt80ColumnsKeepsAggregateAndMonochromeSignals(t *testing.T) {
	t.Parallel()
	output := render(fixtureSnapshot(t), renderOptions{width: 80, height: 24, mono: true, scrollable: true})
	assert.NotContains(t, output, "\x1b[38")
	plain := ansi.Strip(output)
	assert.Contains(t, plain, "99.")
	assert.Contains(t, plain, "▮ maintenance")
	assert.Contains(t, plain, "DEGRADED API Requests")
	assert.NotContains(t, plain, "OPERATIONAL │")
	assert.Contains(t, plain, "API Requests")
	assert.Contains(t, plain, "STATUS HISTORY")
	assert.Contains(t, plain, "Enter")
	assert.Contains(t, plain, "open")
	assert.NotContains(t, plain, "ATOM FEED")
	assert.Contains(t, plain, "CORE SERVICES")
	assert.NotContains(t, plain, "[OK]")
	assert.NotContains(t, plain, "[!]")
	assert.NotContains(t, plain, "3 SERVICES")
	assert.Equal(t, 1, strings.Count(plain, "Git Operations"))
	assert.GreaterOrEqual(t, strings.Count(plain, "▮"), 90)
	assert.NotContains(t, plain, ".m!#X")
	assert.Equal(t, 1, strings.Count(plain, "GITHUB PULSE"))
	assert.NotContains(t, plain, "PLATFORM UPTIME")
}

func TestFormatElapsedUsesCompletedMinutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{name: "future", elapsed: -time.Second, want: "0m"},
		{name: "under minute", elapsed: 59 * time.Second, want: "0m"},
		{name: "minutes", elapsed: 10*time.Minute + 59*time.Second, want: "10m"},
		{name: "hours", elapsed: 90 * time.Minute, want: "1h 30m"},
		{name: "days", elapsed: 51*time.Hour + 4*time.Minute, want: "2d 3h 4m"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, formatElapsed(test.elapsed))
		})
	}
}

func TestIncidentAgeUsesElapsedColorScale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		elapsed time.Duration
		state   pulse.State
	}{
		{name: "under one hour is yellow", elapsed: 59*time.Minute + 59*time.Second, state: pulse.Minor},
		{name: "one hour is orange", elapsed: time.Hour, state: pulse.Major},
		{name: "under three hours stays orange", elapsed: 3*time.Hour - time.Second, state: pulse.Major},
		{name: "three hours is red", elapsed: 3 * time.Hour, state: pulse.Critical},
		{name: "over four hours stays red", elapsed: 5 * time.Hour, state: pulse.Critical},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s := newStyles(false)
			expected := s.muted.Foreground(lipgloss.Color(stateColor(test.state))).Render(formatElapsed(test.elapsed))
			assert.Equal(t, expected, s.incidentAge(test.elapsed))
		})
	}
}

func TestIncidentAgePreservesMonochromeStyle(t *testing.T) {
	t.Parallel()
	s := newStyles(true)
	assert.Equal(t, s.muted.Render("1h 30m"), s.incidentAge(90*time.Minute))
}

func TestRenderContextKeepsIncidentAgeAheadOfOptionalUpdateAt80Columns(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 18, 40, 0, 0, time.UTC)
	started := now.Add(-90 * time.Minute)
	data := pulse.Snapshot{ActiveIncidents: []pulse.Incident{{
		Name:      "Incident with an unusually long Git Operations service name",
		State:     pulse.Major,
		Status:    "monitoring",
		StartedAt: &started,
		LatestUpdate: &pulse.IncidentUpdate{
			Body: "This optional recovery narrative should yield space before the incident age disappears.",
		},
	}}}

	plain := ansi.Strip(renderContext(data, 76, now, newStyles(true)))
	lines := strings.Split(plain, "\n")
	row := lineIndexContaining(lines, "MONITORING")
	require.GreaterOrEqual(t, row, 0)
	line := lines[row]
	assert.Contains(t, line, "Incident with")
	assert.Contains(t, line, "MONITORING")
	assert.Contains(t, line, "1h 30m")
	assert.LessOrEqual(t, ansi.StringWidth(line), 76)
	if update := strings.Index(line, "—"); update >= 0 {
		assert.Less(t, strings.Index(line, "1h 30m"), update)
	}
}

func TestRenderContextOmitsAgeWhenIncidentStartIsMissing(t *testing.T) {
	t.Parallel()
	data := pulse.Snapshot{ActiveIncidents: []pulse.Incident{{
		Name: "Incident without a start", State: pulse.Minor, Status: "investigating",
	}}}

	plain := ansi.Strip(renderContext(data, 76, time.Now(), newStyles(true)))
	assert.Contains(t, plain, "INVESTIGATING")
	assert.NotContains(t, plain, "·  ·")
}

func TestRenderContextStaysWithinTheFortyColumnModelFloor(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 18, 40, 0, 0, time.UTC)
	started := now.Add(-51*time.Hour - 4*time.Minute)
	data := pulse.Snapshot{ActiveIncidents: []pulse.Incident{{
		Name:      "Incident with a name that cannot fit",
		State:     pulse.Major,
		Status:    "investigating",
		StartedAt: &started,
		LatestUpdate: &pulse.IncidentUpdate{
			Body: "Optional update that cannot fit either.",
		},
	}}}

	lines := strings.Split(ansi.Strip(renderContext(data, 36, now, newStyles(true))), "\n")
	for _, line := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(line), 36)
	}
}

func TestRenderAggregateUsesOneHumanAppHeader(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	aggregate := renderAggregate(data, 116, newStyles(true), 42*time.Second)
	plain := strings.ToLower(ansi.Strip(aggregate))
	assert.Contains(t, plain, "github pulse")
	assert.NotContains(t, plain, "●")
	assert.NotContains(t, plain, "all systems operational")
	assert.NotContains(t, plain, "platform uptime")
	assert.NotContains(t, plain, "service health")
	assert.NotContains(t, plain, "live")
	assert.NotContains(t, plain, "ok")
}

func TestRenderAggregateColorsUptimeBadgeFromLiveState(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	data.Overall.State = pulse.Major
	s := newStyles(false)
	s.location = time.UTC

	output := renderAggregate(data, 116, s, 42*time.Second)

	assert.Contains(t, output, s.heroMetric(pulse.Major).Render("99.95%"))
	assert.Contains(t, output, s.state(pulse.Major).Bold(true).Render("100.00%"))
	assert.Contains(t, ansi.Strip(output), "MAJOR SERVICE DISRUPTION")
	assert.NotContains(t, ansi.Strip(output), "●")
}

func TestRenderUnsupportedComponentUsesUnavailableHistory(t *testing.T) {
	t.Parallel()
	s := newStyles(true)
	row := renderComponent(pulse.Component{Name: "Notifications", State: pulse.Operational}, nil, 70, s)
	plain := ansi.Strip(row)
	assert.Contains(t, plain, "Notifications")
	assert.True(t, strings.HasSuffix(plain, "--"))
	assert.NotContains(t, plain, "NO HISTORY")
	assert.Contains(t, row, s.state(pulse.Operational).Bold(true).Render("OK"))
	assert.NotContains(t, strings.ToLower(plain), "operational")
	assert.NotContains(t, row, stateBar(pulse.Unknown, s))
}

func TestRenderComponentsKeepsUngroupedServicesOutsideGroupSections(t *testing.T) {
	t.Parallel()
	groupID := "core"
	components := []pulse.Component{
		{ID: "empty", Name: "Empty group", Group: true},
		{ID: groupID, Name: "Core services", Group: true},
		{Name: "Git Operations", State: pulse.Operational, GroupID: &groupID},
		{Name: "API Requests", State: pulse.Operational},
		{Name: "Notifications", State: pulse.Operational},
	}
	lines := strings.Split(ansi.Strip(renderComponents(components, nil, 120, newStyles(true))), "\n")
	ungroupedLine := lineIndexContaining(lines, "API Requests")
	notificationLine := lineIndexContaining(lines, "Notifications")
	groupLine := lineIndexContaining(lines, "CORE SERVICES")
	childLine := lineIndexContaining(lines, "Git Operations")

	require.GreaterOrEqual(t, ungroupedLine, 0)
	require.GreaterOrEqual(t, notificationLine, 0)
	require.GreaterOrEqual(t, groupLine, 0)
	require.GreaterOrEqual(t, childLine, 0)
	assert.Less(t, ungroupedLine, groupLine)
	assert.Less(t, notificationLine, groupLine)
	assert.Less(t, groupLine, childLine)
	assert.NotContains(t, strings.Join(lines, "\n"), "EMPTY GROUP")
}

func TestComponentHistorySeparatesCurrentStateFromRecentDailyCells(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	plain := ansi.Strip(renderComponents(data.Components, data.History.Components, 116, newStyles(true)))
	assert.Contains(t, plain, "NOW")
	assert.Contains(t, plain, "COMPONENT")
	assert.Contains(t, plain, "90D UPTIME")
	assert.Contains(t, plain, "30D HISTORY")
	assert.NotContains(t, plain, "RECENT DAYS")
}

func TestComponentGridUsesFullWidthInTwoColumnLayout(t *testing.T) {
	t.Parallel()
	components := []pulse.Component{
		{Name: "Git Operations", State: pulse.Operational},
		{Name: "Webhooks", State: pulse.Operational},
	}
	const width = 136
	lines := strings.Split(ansi.Strip(renderComponentGrid(components, nil, width, newStyles(true))), "\n")
	require.Len(t, lines, 2)
	for _, line := range lines {
		assert.Equal(t, width, ansi.StringWidth(line))
	}
}

func TestComponentGridKeepsLongServiceNamesAtStandardWidth(t *testing.T) {
	t.Parallel()
	uptime := 99.88
	days := make([]pulse.Day, 30)
	for index := range days {
		days[index].State = pulse.Operational
	}
	components := []pulse.Component{
		{Name: "Git Operations", State: pulse.Operational},
		{Name: "Copilot AI Model Providers", State: pulse.Operational},
	}
	histories := []pulse.ComponentHistory{{
		Name: "Copilot AI Model Providers", Uptime90Days: &uptime, Days90: days,
	}}
	plain := ansi.Strip(renderComponentGrid(components, histories, 112, newStyles(true)))

	assert.Contains(t, plain, "Copilot AI Model Providers")
	assert.NotContains(t, plain, "Copilot AI Mo…")
	assert.Contains(t, plain, "30D HISTORY")
}

func TestComponentGridHeadersAlignWithValues(t *testing.T) {
	t.Parallel()
	uptime := 99.87
	days := make([]pulse.Day, 30)
	for index := range days {
		days[index].State = pulse.Operational
	}
	components := []pulse.Component{{Name: "Git Operations", State: pulse.Operational}}
	histories := []pulse.ComponentHistory{{Name: "Git Operations", Uptime90Days: &uptime, Days90: days}}
	lines := strings.Split(ansi.Strip(renderComponentGrid(components, histories, 70, newStyles(true))), "\n")
	require.Len(t, lines, 2)

	assert.Equal(t, ansi.StringWidth(strings.SplitN(lines[0], "COMPONENT", 2)[0]), ansi.StringWidth(strings.SplitN(lines[1], "Git Operations", 2)[0]))
	headerUptimeEnd := ansi.StringWidth(strings.SplitN(lines[0], "90D UPTIME", 2)[0]) + len("90D UPTIME")
	valueUptimeEnd := ansi.StringWidth(strings.SplitN(lines[1], "99.87%", 2)[0]) + len("99.87%")
	assert.Equal(t, headerUptimeEnd, valueUptimeEnd)
	assert.Equal(t, ansi.StringWidth(strings.SplitN(lines[0], "30D HISTORY", 2)[0]), ansi.StringWidth(strings.SplitN(lines[1], "▮", 2)[0]))
}

func TestComponentHistoryShowsThirtyDaysEndingAtLatestCompleteDay(t *testing.T) {
	t.Parallel()
	s := newStyles(false)
	days := make([]pulse.Day, 90)
	for index := range days {
		days[index].State = pulse.Operational
	}
	days[len(days)-2].State = pulse.Critical
	history := &pulse.ComponentHistory{Days90: days}

	row := renderComponent(pulse.Component{Name: "Actions", State: pulse.Operational}, history, 80, s)

	assert.Equal(t, 30, strings.Count(ansi.Strip(row), "▮"))
	assert.True(t, strings.HasSuffix(row, stateBar(pulse.Operational, s)))
}

func TestComponentHistoryPreservesThirtyDayCoverageWhenCellsCompress(t *testing.T) {
	t.Parallel()
	s := newStyles(false)
	days := make([]pulse.Day, 90)
	for index := range days {
		days[index].State = pulse.Operational
	}
	days[len(days)-30].State = pulse.Critical
	history := &pulse.ComponentHistory{Days90: days}

	row := renderComponent(pulse.Component{Name: "Actions", State: pulse.Operational}, history, 40, s)

	assert.Contains(t, row, stateBar(pulse.Critical, s))
}

func TestAggregateCombinesDailyAndRollingHistory(t *testing.T) {
	t.Parallel()
	output := render(fixtureSnapshot(t), renderOptions{width: 120, height: 40, mono: true})
	assert.NotContains(t, output, "\t")
	for _, value := range []string{"SINCE 2022", "⯅", "⯆", "90-DAY ROLLING", "DAYS", "█"} {
		assert.Contains(t, output, value)
	}
	assert.NotContains(t, output, "BEST 90D")
	assert.NotContains(t, output, "WORST 90D")
	assert.NotContains(t, output, "CURRENT 90D")
	assert.NotContains(t, output, "PEAK")
	assert.NotContains(t, output, "LOW")
	assert.NotContains(t, output, "LIFETIME")
}

func TestWideHistoryStripsShareOneRow(t *testing.T) {
	t.Parallel()
	lines := strings.Split(ansi.Strip(render(fixtureSnapshot(t), renderOptions{width: 120, height: 40, mono: true})), "\n")
	dailyStrip := lineIndexContaining(lines, strings.Repeat("▮", 20))
	rollingSparkline := lineIndexContaining(lines, "█")
	require.GreaterOrEqual(t, dailyStrip, 0)
	require.GreaterOrEqual(t, rollingSparkline, 0)
	assert.Equal(t, dailyStrip, rollingSparkline)
}

func TestWideRollingDetailsSitDirectlyUnderSparkline(t *testing.T) {
	t.Parallel()
	lines := strings.Split(ansi.Strip(renderCombinedHistory(fixtureSnapshot(t).History, 116, newStyles(true))), "\n")
	divider := lineIndexContaining(lines, "─")
	sparkline := lineIndexContaining(lines, "█")
	require.GreaterOrEqual(t, divider, 0)
	require.GreaterOrEqual(t, sparkline, 1)
	require.Less(t, sparkline+1, len(lines))
	require.Less(t, divider, sparkline-1)
	assert.Contains(t, lines[sparkline-1], "90-DAY ROLLING")
	assert.Contains(t, lines[sparkline-1], "⯆ ~0 DAYS")
	assert.Contains(t, lines[sparkline+1], "⯅")
	assert.Equal(t, 1, strings.Count(lines[sparkline+1], "⯆"))
	assert.Contains(t, lines[sparkline+1], "2022-09-09")
	assert.Contains(t, lines[sparkline+1], "2026-08-08")
	sparkStart := ansi.StringWidth(strings.SplitN(lines[sparkline], "█", 2)[0])
	headerStart := ansi.StringWidth(strings.SplitN(lines[sparkline-1], "90-DAY ROLLING", 2)[0])
	detailsStart := ansi.StringWidth(strings.SplitN(lines[sparkline+1], "⯅", 2)[0])
	assert.Equal(t, sparkStart, headerStart)
	assert.Equal(t, sparkStart, detailsStart)
}

func TestRollingHeaderRoundsEquivalentDowntimeToWholeDays(t *testing.T) {
	t.Parallel()
	history := fixtureSnapshot(t).History
	downtimeDays := 4.6
	history.Downtime90Days = &downtimeDays

	header := ansi.Strip(renderRollingHeader(history, 44, newStyles(true)))
	details := ansi.Strip(renderRollingDetails(history, 44, newStyles(true)))

	assert.Contains(t, header, "90-DAY ROLLING")
	assert.Contains(t, header, "⯆ ~5 DAYS")
	assert.Contains(t, details, "⯅")
	assert.Equal(t, 1, strings.Count(details, "⯆"))
	assert.Contains(t, details, "2022-09-09")
	assert.Contains(t, details, "2026-08-08")
	assert.Len(t, strings.Split(details, "\n"), 1)
	assert.NotContains(t, details, "SINCE")
	assert.NotContains(t, details, "PEAK")
	assert.NotContains(t, details, "LOW")
}

func TestCombinedHistoryKeepsBothChartsAtEveryWidth(t *testing.T) {
	t.Parallel()
	history := fixtureSnapshot(t).History
	for _, width := range []int{76, 112, 136} {
		output := ansi.Strip(renderCombinedHistory(history, width, newStyles(true)))
		assert.NotContains(t, output, "SINCE 2022")
		assert.NotContains(t, output, "BEST 90D")
		assert.NotContains(t, output, "WORST 90D")
		assert.NotContains(t, output, "CURRENT 90D")
		assert.Contains(t, output, "▮")
		assert.Contains(t, output, "█")
		assert.NotContains(t, output, "COMPLETE UTC")
	}
}

func TestRenderUsesDesignedStatusSurfaces(t *testing.T) {
	t.Parallel()
	plain := ansi.Strip(render(fixtureSnapshot(t), renderOptions{width: 120, height: 40, mono: true}))
	assert.Contains(t, plain, "╭")
	assert.Contains(t, plain, "GITHUB PULSE")
	assert.Contains(t, plain, "LAST 90 DAYS")
	assert.Contains(t, plain, "2026-08-08")
	assert.Contains(t, plain, "▮ operational")
	assert.Contains(t, plain, "99.95%")
}

func TestRenderKeepsUptimeMetricProminentUnderAppHeader(t *testing.T) {
	t.Parallel()
	plain := ansi.Strip(render(fixtureSnapshot(t), renderOptions{width: 120, height: 40, mono: true}))
	lines := strings.Split(plain, "\n")
	header := lineIndexContaining(lines, "GITHUB PULSE")
	require.GreaterOrEqual(t, header, 0)
	assert.Equal(t, 1, strings.Count(plain, "GITHUB PULSE"))
	assert.NotContains(t, plain, "PLATFORM UPTIME")
	assert.Contains(t, lines[header], "GITHUB PULSE │ 90D UPTIME")
	assert.Regexp(t, `90D UPTIME +[0-9]+\.[0-9]{2}%`, lines[header])
	assert.Regexp(t, `ALL TIME UPTIME +[0-9]+\.[0-9]{2}% SINCE 2022`, lines[header])
	assert.NotContains(t, lines[header], "UPTIME:")
}

func TestRenderKeepsCriticalHeaderOnOneRowAtSupportedWidths(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	data.Overall.State = pulse.Critical

	for _, width := range []int{80, 120, 200} {
		t.Run(fmt.Sprintf("%d columns", width), func(t *testing.T) {
			plain := ansi.Strip(render(data, renderOptions{width: width, height: 50, mono: true, countdown: 49 * time.Second}))
			lines := strings.Split(plain, "\n")
			headerLine := lineIndexContaining(lines, "GITHUB PULSE")
			refreshLine := lineIndexContaining(lines, "↻ 00:49")

			require.GreaterOrEqual(t, headerLine, 0)
			require.GreaterOrEqual(t, refreshLine, 0)
			require.Equal(t, headerLine, refreshLine)
			header := lines[headerLine]
			assert.Contains(t, header, "OUTAGE")
			assert.Contains(t, header, "90D")
			assert.Contains(t, header, "99.95%")
			assert.Contains(t, header, "ALL")
			assert.Contains(t, header, "100.00%")
			assert.Regexp(t, `SINCE (2022|'22)`, header)
			assert.Regexp(t, `[0-9]{2}:[0-9]{2} ([A-Z]+|[+-][0-9]{4})`, header)
		})
	}
}

func TestRenderAggregateKeepsWorstCaseCompactHeaderComplete(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	data.Overall.State = pulse.Minor
	percent := 100.0
	data.History.Uptime90Days = &percent
	data.History.TrackedUptime = &percent
	s := newStyles(true)
	s.location = time.FixedZone("CEST", 2*60*60)

	plain := ansi.Strip(renderAggregate(data, 76, s, 49*time.Second))
	lines := strings.Split(plain, "\n")
	headerLine := lineIndexContaining(lines, "GITHUB PULSE")
	refreshLine := lineIndexContaining(lines, "↻ 00:49")
	require.GreaterOrEqual(t, headerLine, 0)
	require.Equal(t, headerLine, refreshLine)
	header := lines[headerLine]
	assert.Regexp(t, `(DEGRADED|MINOR)`, header)
	assert.Equal(t, 2, strings.Count(header, "100.00%"))
	assert.Regexp(t, `SINCE (2022|'22)`, header)
	assert.Contains(t, header, "16:00 CEST")
	assert.NotContains(t, header, "…")
}

func TestRenderFeedHonorsRowCapacity(t *testing.T) {
	t.Parallel()
	value := fixtureSnapshot(t)
	value.RecentFeed = []pulse.FeedEntry{
		{Title: "First incident", UpdatedAt: value.GeneratedAt},
		{Title: "Second incident", UpdatedAt: value.GeneratedAt},
		{Title: "Third incident", UpdatedAt: value.GeneratedAt},
		{Title: "Fourth incident", UpdatedAt: value.GeneratedAt},
		{Title: "Fifth incident", UpdatedAt: value.GeneratedAt},
		{Title: "Sixth incident", UpdatedAt: value.GeneratedAt},
	}
	short := ansi.Strip(renderFeed(value, 140, 1, 0, 0, newStyles(true)))
	tall := ansi.Strip(renderFeed(value, 116, 5, 0, 0, newStyles(true)))
	assert.Contains(t, short, "First incident")
	assert.NotContains(t, short, "Second incident")
	for _, title := range []string{"First incident", "Second incident", "Third incident", "Fourth incident", "Fifth incident"} {
		assert.Contains(t, tall, title)
	}
	assert.NotContains(t, tall, "Sixth incident")
}

func TestRenderFeedReadsAsDatedHistory(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	data.RecentFeed = append(data.RecentFeed,
		pulse.FeedEntry{Title: "Incident with Actions", UpdatedAt: data.RecentFeed[0].UpdatedAt.Add(-time.Hour)},
		pulse.FeedEntry{Title: "Incident with Copilot", UpdatedAt: data.RecentFeed[0].UpdatedAt.Add(-2 * time.Hour)},
	)
	s := newStyles(true)
	s.location = time.FixedZone("LOCAL", -4*60*60)
	plain := ansi.Strip(renderFeed(data, 116, len(data.RecentFeed), 0, 0, s))
	assert.Contains(t, plain, "STATUS HISTORY")
	assert.Contains(t, plain, "2026-08-09 10:00 LOCAL")
	assert.NotContains(t, plain, "●")
}

func TestRenderFeedLinksIncidentTitlesWhenDetailsURLExists(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	target := "https://www.githubstatus.com/incidents/example"
	data.RecentFeed[0].URL = &target

	output := renderFeed(data, 116, len(data.RecentFeed), 0, 0, newStyles(true))

	assert.Contains(t, output, ansi.SetHyperlink(target))
	assert.Contains(t, output, "Incident with API Requests")
	assert.Contains(t, output, ansi.ResetHyperlink())
}

func TestTerminalLinkRejectsControlSequences(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "incident", terminalLink("incident", "https://example.com/\x1b]8;;evil"))
	assert.Equal(t, "incident", terminalLink("incident", "https://example.com/\u009cspoof"))
	assert.Equal(t, "incident", terminalLink("incident", "https://example.com/\u202espoof"))
	linked := terminalLink("incident\x1b\n\tspoof", "https://example.com/status")
	assert.Equal(t, "incidentspoof", ansi.Strip(linked))
	assert.NotContains(t, linked, "\x1bspoof")
}

func TestRenderFeedStripsControlsFromUnlinkedTitles(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	data.RecentFeed[0].Title = "incident\u009cspoof\u202e"

	output := renderFeed(data, 116, len(data.RecentFeed), 0, 0, newStyles(true))

	assert.Contains(t, output, "incidentspoof")
	assert.NotContains(t, output, "\u009c")
	assert.NotContains(t, output, "\u202e")
}

func TestRenderDoesNotWrapPanelMetadataIntoOrphanLines(t *testing.T) {
	t.Parallel()
	plain := ansi.Strip(render(fixtureSnapshot(t), renderOptions{width: 120, height: 40, mono: true}))
	for _, line := range strings.Split(plain, "\n") {
		assert.NotContains(t, []string{"TIME", "UTC", "DAYS", "FEED"}, strings.TrimSpace(line))
	}
}

func TestSparklineUsesReferenceVerticalGradient(t *testing.T) {
	t.Parallel()
	assert.Equal(t, stateColor(pulse.Critical), sparklineColor(0))
	assert.Equal(t, stateColor(pulse.Major), sparklineColor(1))
	assert.Equal(t, stateColor(pulse.Minor), sparklineColor(2))
	assert.Equal(t, stateColor(pulse.Operational), sparklineColor(7))
}

func TestSparklineUsesAbsoluteLevelForFlatHistory(t *testing.T) {
	t.Parallel()
	perfect := []pulse.RollingPoint{{Uptime: 100}, {Uptime: 100}}
	failed := []pulse.RollingPoint{{Uptime: 0}, {Uptime: 0}}
	assert.Equal(t, "██", ansi.Strip(renderSparkline(perfect, 2, newStyles(false))))
	assert.Equal(t, "▁▁", ansi.Strip(renderSparkline(failed, 2, newStyles(false))))
}

func TestNinetyDayAnchorsEndWithStrip(t *testing.T) {
	t.Parallel()
	history := fixtureSnapshot(t).History
	ninetyDayLines := strings.Split(ansi.Strip(render90DayTimeline(history, 90, newStyles(true))), "\n")
	anchor := lineIndexContaining(ninetyDayLines, "LAST 90 DAYS")
	require.GreaterOrEqual(t, anchor, 0)
	require.Less(t, anchor+1, len(ninetyDayLines))
	assert.Contains(t, ninetyDayLines[anchor], "2026-08-08")
	assert.NotContains(t, ninetyDayLines[anchor], "TODAY")
	assert.Equal(t, 90, ansi.StringWidth(ninetyDayLines[anchor]))
	assert.Equal(t, ansi.StringWidth(ninetyDayLines[anchor]), ansi.StringWidth(ninetyDayLines[anchor+1]))
}

func TestRollingDowntimeUsesCurrentNinetyDayWindow(t *testing.T) {
	t.Parallel()
	history := fixtureSnapshot(t).History
	output := ansi.Strip(renderCombinedHistory(history, 116, newStyles(true)))
	assert.Contains(t, output, "⯆ ~0 DAYS")
	assert.NotContains(t, output, "SINCE 2022-06-11")
}

func lineIndexContaining(lines []string, value string) int {
	for index, line := range lines {
		if strings.Contains(line, value) {
			return index
		}
	}
	return -1
}

func TestRenderShowsSourceErrorsAndMaintenanceBounds(t *testing.T) {
	t.Parallel()
	value := fixtureSnapshot(t)
	start := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	value.GeneratedAt = time.Date(2026, 8, 9, 14, 5, 0, 0, time.UTC)
	attempted := value.GeneratedAt
	value.ActiveMaintenances = []pulse.MaintenanceWindow{{
		Name: "Database maintenance", State: pulse.Maintenance, Status: "in_progress",
		ScheduledFor: &start, ScheduledUntil: &end,
	}}
	value.Errors = []pulse.SourceError{{Source: "feed", Message: "request timed out", AttemptedAt: &attempted}}

	plain := ansi.Strip(render(value, renderOptions{width: 120, height: 40, mono: true}))
	assert.Contains(t, plain, start.Local().Format("15:04 MST")+"–"+end.Local().Format("15:04 MST"))
	assert.Contains(t, plain, "SOURCE ISSUES")
	assert.Contains(t, plain, "feed: request timed out")
	assert.Contains(t, plain, "attempt "+attempted.Local().Format("15:04 MST"))
}

func TestMaintenanceBoundsDisambiguatesOffsetChanges(t *testing.T) {
	t.Parallel()
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	start := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	end := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	s := newStyles(true)
	s.location = location

	got := maintenanceBounds(pulse.MaintenanceWindow{ScheduledFor: &start, ScheduledUntil: &end}, s)

	assert.Equal(t, "2026-11-01 01:30 EDT–2026-11-01 01:30 EST", got)
}

func TestRenderUsesComputerTimezoneForTimestamps(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	updated := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	data.Overall.UpdatedAt = &updated
	s := newStyles(true)
	s.location = time.FixedZone("LOCAL", -4*60*60)

	assert.Contains(t, ansi.Strip(renderAggregate(data, 116, s, 42*time.Second)), "UPDATED 10:00 LOCAL")
	assert.Contains(t, ansi.Strip(renderFeed(data, 116, len(data.RecentFeed), 0, 0, s)), "2026-08-09 10:00 LOCAL")
}

func TestRenderAggregateShowsRefreshCountdown(t *testing.T) {
	t.Parallel()
	data := fixtureSnapshot(t)
	s := newStyles(true)
	assert.Contains(t, ansi.Strip(renderAggregate(data, 116, s, 42*time.Second)), "↻ 00:42")
}

type fetcherStub struct{ value pulse.Snapshot }

func (f fetcherStub) Fetch(context.Context) pulse.Snapshot     { return f.value }
func (f fetcherStub) FetchLive(context.Context) pulse.Snapshot { return f.value }

type barrierFetcher struct {
	data    pulse.Snapshot
	started chan struct{}
	release chan struct{}
}

func (f barrierFetcher) Fetch(context.Context) pulse.Snapshot {
	close(f.started)
	<-f.release
	return f.data
}

func (f barrierFetcher) FetchLive(context.Context) pulse.Snapshot { return f.data }

func TestModelPublishesInitialSourcesAtomically(t *testing.T) {
	t.Parallel()
	fetcher := barrierFetcher{
		data: fixtureSnapshot(t), started: make(chan struct{}), release: make(chan struct{}),
	}
	model := New(fetcher, true)
	clock := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	model.now = func() time.Time { return clock }
	model.width, model.height = 120, 80
	model.nextRefresh = clock.Add(10 * time.Second)
	command := model.startFullRefresh()
	model.syncView()
	loading := ansi.Strip(model.View().Content)
	assert.Empty(t, strings.TrimSpace(loading))
	assert.NotContains(t, loading, "COMPONENTS")

	result := make(chan tea.Msg, 1)
	go func() { result <- command() }()
	<-fetcher.started
	select {
	case <-result:
		t.Fatal("full refresh completed before history was ready")
	default:
	}
	assert.False(t, model.ready)

	close(fetcher.release)
	updated, _ := model.Update(<-result)
	model = updated.(*Model)
	assert.True(t, model.ready)
	assert.Equal(t, clock.Add(refreshEvery), model.nextRefresh)
	plain := ansi.Strip(model.View().Content)
	assert.Contains(t, plain, "GITHUB PULSE")
	assert.Contains(t, plain, "COMPONENTS")
	assert.Contains(t, plain, "STATUS HISTORY")
}

func TestModelResizePreservesViewportDimensions(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{fixtureSnapshot(t)}, true)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(*Model)
	assert.Equal(t, 120, model.view.Width())
	assert.Equal(t, 40, model.view.Height())
}

func TestModelKeepsInitialScreenBlankAndManualRefreshStable(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{fixtureSnapshot(t)}, true)
	model.Init()
	t.Cleanup(model.cancelAll)
	initial := ansi.Strip(model.View().Content)
	assert.Empty(t, strings.TrimSpace(initial))
	assert.NotContains(t, initial, "COMPONENTS")

	model.ready = true
	model.data = fixtureSnapshot(t)
	clock := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	model.now = func() time.Time { return clock }
	model.nextRefresh = clock.Add(30 * time.Second)
	model.fullLoading = false
	model.syncView()
	before := model.View().Content
	updated, command := model.Update(tea.KeyPressMsg{Code: 'r'})
	model = updated.(*Model)
	require.NotNil(t, command)
	assert.Equal(t, before, model.View().Content)
}

func TestModelCountdownTicksWithoutRefreshingEarly(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	model := New(fetcherStub{fixtureSnapshot(t)}, true)
	model.now = func() time.Time { return clock }
	model.ready = true
	model.data = fixtureSnapshot(t)
	model.nextRefresh = clock.Add(refreshEvery)
	model.syncView()
	assert.Contains(t, ansi.Strip(model.View().Content), "↻ 01:00")

	clock = clock.Add(time.Second)
	updated, command := model.Update(tickMsg(clock))
	model = updated.(*Model)
	require.NotNil(t, command)
	assert.Contains(t, ansi.Strip(model.View().Content), "↻ 00:59")
	assert.Equal(t, uint64(0), model.liveGen)

	clock = model.nextRefresh
	updated, command = model.Update(tickMsg(clock))
	model = updated.(*Model)
	t.Cleanup(model.cancelAll)
	require.NotNil(t, command)
	assert.Equal(t, uint64(1), model.liveGen)
}

func TestModelTickAdvancesIncidentAgeWithoutRefreshingEarly(t *testing.T) {
	t.Parallel()
	tickAt := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	clock := tickAt.Add(-time.Second)
	started := tickAt.Add(-90 * time.Minute)
	data := fixtureSnapshot(t)
	data.ActiveIncidents = []pulse.Incident{{
		Name: "Incident with API Requests", State: pulse.Major, Status: "monitoring", StartedAt: &started,
	}}
	model := New(fetcherStub{data}, true)
	model.now = func() time.Time { return clock }
	model.ready = true
	model.data = data
	model.nextRefresh = tickAt.Add(30 * time.Second)
	model.syncView()
	assert.Contains(t, ansi.Strip(model.View().Content), "1h 29m")

	updated, command := model.Update(tickMsg(tickAt))
	model = updated.(*Model)
	require.NotNil(t, command)
	assert.Contains(t, ansi.Strip(model.View().Content), "1h 30m")
	assert.Equal(t, uint64(0), model.liveGen)
	assert.Equal(t, uint64(0), model.fullGen)
}

func TestModelCentersContentVerticallyWhenItFits(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{fixtureSnapshot(t)}, true)
	model.ready = true
	model.data = fixtureSnapshot(t)
	model.width, model.height = 200, 80
	model.syncView()
	lines := strings.Split(model.view.View(), "\n")
	require.NotEmpty(t, lines)
	assert.Empty(t, strings.TrimSpace(lines[0]))
}

func TestModelAdvertisesScrollOnlyWhenContentOverflows(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	model.data = fixtureSnapshot(t)
	model.ready = true
	model.width, model.height = 120, 80
	model.syncView()
	assert.NotContains(t, ansi.Strip(model.view.View()), "scroll")

	model.height = 20
	model.syncView()
	model.view.GotoBottom()
	assert.Contains(t, ansi.Strip(model.view.View()), "scroll")
}

func TestRenderFooterKeepsScrollDiscoverableAtMinimumWidth(t *testing.T) {
	t.Parallel()
	footer := strings.Split(ansi.Strip(renderFooter(36, newStyles(true), true, true)), "\n")
	require.NotEmpty(t, footer)
	assert.Contains(t, footer[0], "scroll")
	assert.Contains(t, footer[0], "j/k")
	assert.NotContains(t, footer[0], "↑/↓ scroll")
	assert.NotContains(t, footer[0], "…")
}

func TestRenderFooterContainsOnlyKeyboardShortcuts(t *testing.T) {
	t.Parallel()
	footer := ansi.Strip(renderFooter(116, newStyles(true), false, true))
	assert.Len(t, strings.Split(footer, "\n"), 1)
	assert.Contains(t, footer, "quit")
	assert.Contains(t, footer, "select")
	assert.Contains(t, footer, "open")
	assert.Contains(t, footer, "refresh")
	assert.NotContains(t, footer, "range")
	assert.NotContains(t, footer, "tab")
	assert.NotContains(t, footer, "CHECKED")
}

func TestModelRefreshesHistoryAfterUTCDateChanges(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	fetchedAt := time.Date(2026, 8, 9, 23, 59, 0, 0, time.UTC)
	model.data.Sources.History.FetchedAt = &fetchedAt
	model.nextRefresh = fetchedAt.Add(time.Minute)

	_, command := model.Update(tickMsg(fetchedAt.Add(time.Minute)))
	t.Cleanup(model.cancelAll)

	require.NotNil(t, command)
	assert.Equal(t, uint64(1), model.fullGen)
	assert.True(t, model.fullLoading)
}

func TestModelRetriesRolloverAfterTransientHistoryFailure(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	lastSuccess := time.Date(2026, 8, 9, 23, 59, 0, 0, time.UTC)
	failedAttempt := lastSuccess.Add(time.Minute)
	model.data.Sources.History.FetchedAt = &lastSuccess
	model.errors["history"] = pulse.SourceError{Source: "history", Message: "temporary failure", AttemptedAt: &failedAttempt}

	assert.True(t, model.historyNeedsRefresh(failedAttempt.Add(time.Minute)))
}

func TestModelRetriesHistoryThatHasNeverSucceeded(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	attempted := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	model.errors["history"] = pulse.SourceError{Source: "history", Message: "temporary failure", AttemptedAt: &attempted}

	assert.True(t, model.historyNeedsRefresh(attempted.Add(time.Minute)))
}

type cancelFetcher struct {
	started, canceled chan struct{}
}

func (f cancelFetcher) Fetch(ctx context.Context) pulse.Snapshot {
	close(f.started)
	<-ctx.Done()
	close(f.canceled)
	return pulse.Snapshot{}
}

func (f cancelFetcher) FetchLive(context.Context) pulse.Snapshot { return pulse.Snapshot{} }

func TestModelCtrlCQuitsAndCancelsRequests(t *testing.T) {
	t.Parallel()
	fetcher := cancelFetcher{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	model := New(fetcher, true)
	refresh := model.startFullRefresh()
	go refresh()
	<-fetcher.started

	_, command := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	require.NotNil(t, command)
	assert.Equal(t, tea.Quit(), command())
	assert.Eventually(t, func() bool {
		select {
		case <-fetcher.canceled:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}

func TestTruncateRespectsTerminalCellWidth(t *testing.T) {
	t.Parallel()
	got := truncate("界界界", 5)
	assert.LessOrEqual(t, ansi.StringWidth(got), 5)
	assert.Contains(t, got, "…")
}

func TestModelDiscardsStaleLiveResults(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	model.liveGen = 2
	model.data.Overall = pulse.Overall{State: pulse.Operational}

	updated, _ := model.Update(liveMsg{generation: 1, value: pulse.Snapshot{
		Overall: pulse.Overall{State: pulse.Critical},
		Sources: pulse.Sources{Current: pulse.Source{Available: true}},
	}})

	assert.Equal(t, pulse.Operational, updated.(*Model).data.Overall.State)
}

func TestModelKeepsLastGoodLiveDataAndScopedErrors(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	model.liveGen = 2
	fetchedAt := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	model.data.Overall = pulse.Overall{State: pulse.Operational}
	model.data.Sources.Current = pulse.Source{Available: true, FetchedAt: &fetchedAt}
	model.errors["history"] = pulse.SourceError{Source: "history", Message: "history unavailable"}

	updated, _ := model.Update(liveMsg{generation: 2, value: pulse.Snapshot{
		Overall: pulse.Overall{State: pulse.Unknown},
		Sources: pulse.Sources{},
		Errors:  []pulse.SourceError{{Source: "current", Message: "current unavailable"}},
	}})
	got := updated.(*Model)

	assert.Equal(t, pulse.Operational, got.data.Overall.State)
	assert.Equal(t, &fetchedAt, got.data.Sources.Current.FetchedAt)
	assert.Equal(t, []pulse.SourceError{
		{Source: "current", Message: "current unavailable"},
		{Source: "history", Message: "history unavailable"},
	}, got.data.Errors)
}

func TestModelHistoryErrorRetainsItsOwnAttemptTime(t *testing.T) {
	t.Parallel()
	model := New(fetcherStub{}, true)
	model.fullGen = 1
	attempted := time.Date(2026, 8, 9, 14, 5, 0, 0, time.UTC)

	updated, _ := model.Update(fullMsg{
		generation: 1,
		value: pulse.Snapshot{
			GeneratedAt: attempted,
			Sources:     pulse.Sources{Current: pulse.Source{Available: true}},
			Errors:      []pulse.SourceError{{Source: "history", Message: "history unavailable", AttemptedAt: &attempted}},
		},
	})
	model = updated.(*Model)
	model.liveGen = 2
	later := attempted.Add(time.Minute)
	updated, _ = model.Update(liveMsg{generation: 2, value: pulse.Snapshot{
		GeneratedAt: later,
		Sources:     pulse.Sources{Current: pulse.Source{Available: true}},
	}})

	require.Len(t, updated.(*Model).data.Errors, 1)
	require.NotNil(t, updated.(*Model).data.Errors[0].AttemptedAt)
	assert.Equal(t, attempted, *updated.(*Model).data.Errors[0].AttemptedAt)
}
