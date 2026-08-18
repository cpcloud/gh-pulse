// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntryMarkdownPreservesStructureWithoutTerminalInjection(t *testing.T) {
	t.Parallel()
	input := `<h2>Incident update</h2>
<p>A&#27;B&#x202e;C <strong>bold</strong>
<a href="javascript:alert(1)">unsafe script</a>
<a href="data:text/plain,unsafe">unsafe data</a>
<a href="https://www.githubstatus.com/incidents/example">safe details</a></p>
<ul><li>First item</li><li>Second item</li></ul>
<script>script payload</script>`

	got, err := entryMarkdown(input, time.Time{}, time.UTC)

	require.NoError(t, err)
	assert.Contains(t, got, "## Incident update")
	assert.Contains(t, got, "**bold**")
	assert.Contains(t, got, "[safe details](https://www.githubstatus.com/incidents/example)")
	assert.Contains(t, got, "- First item")
	assert.NotContains(t, got, "\x1b")
	assert.NotContains(t, got, "\u202e")
	assert.NotContains(t, got, "javascript:")
	assert.NotContains(t, got, "data:")
	assert.NotContains(t, got, "script payload")
}

func TestRenderEntryBodyAllowsOnlyHTTPHyperlinks(t *testing.T) {
	t.Parallel()
	body := renderEntryBody(
		`<p>ftp://files.example.com ops@example.com <a href="https://www.githubstatus.com">safe details</a></p>`,
		time.Time{},
		80,
		false,
		newStyles(false),
	)

	assert.NotContains(t, body, ";ftp://")
	assert.NotContains(t, body, ";mailto:")
	assert.Contains(t, body, ";https://www.githubstatus.com")
}

func TestRenderEntryBodySeparatesFlushLeftParagraphs(t *testing.T) {
	t.Parallel()
	body := ansi.Strip(renderEntryBody(
		"<p>First update</p><p>Second update</p>",
		time.Time{},
		68,
		true,
		newStyles(true),
	))

	lines := strings.Split(strings.TrimSpace(body), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " ")
	}
	require.Equal(t, []string{"First update", strings.Repeat("─", 68), "Second update"}, lines)
}

func TestRenderEntryBodyUsesTerminalEmphasisInMonochrome(t *testing.T) {
	t.Parallel()
	body := renderEntryBody("<p><strong>Important update</strong></p>", time.Time{}, 68, true, newStyles(true))

	assert.Contains(t, ansi.Strip(body), "Important update")
	assert.NotContains(t, body, "**")
	assert.Contains(t, body, "\x1b[1m")
}

func TestRenderEntryBodyUsesTerminalHeadingStyleInMonochrome(t *testing.T) {
	t.Parallel()
	body := renderEntryBody("<h2>Incident update</h2>", time.Time{}, 68, true, newStyles(true))

	assert.Contains(t, ansi.Strip(body), "Incident update")
	assert.NotContains(t, body, "##")
	assert.Contains(t, body, "\x1b[1m")
}

func TestRenderEntryContentLocalizesSplitUTCTimestampsWithoutCodeStyling(t *testing.T) {
	t.Parallel()
	styles := newStyles(false)
	styles.location = time.FixedZone("EDT", -4*60*60)
	entry := pulse.FeedEntry{
		Title:       "Incident with Actions",
		ContentHTML: `<p>Aug <var>18</var>, <var>10:23</var> UTC <strong>Resolved</strong> - Workflows were not impacted.</p>`,
		UpdatedAt:   time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC),
	}

	plain := ansi.Strip(renderEntryContent(entry, 116, false, styles))

	assert.Contains(t, plain, "Aug 18, 06:23 EDT")
	assert.NotContains(t, plain, "10:23 UTC")
	assert.NotContains(t, plain, "\u00a0")
	assert.NotContains(t, plain, "``")
	assert.Contains(t, plain, "Resolved")
}

func TestRenderEntryContentUsesReadableUpdateTable(t *testing.T) {
	t.Parallel()
	styles := newStyles(true)
	styles.location = time.FixedZone("EDT", -4*60*60)
	entry := pulse.FeedEntry{
		Title: "Incident with Actions",
		ContentHTML: `<p>Aug <var>18</var>, <var>10:23</var> UTC <strong>Resolved</strong> - Workflows recovered normally.</p>` +
			`<p>Aug <var>18</var>, <var>09:36</var> UTC <strong>Investigating</strong> - Engineers investigated affected services.</p>`,
		UpdatedAt: time.Date(2026, 8, 18, 10, 30, 0, 0, time.UTC),
	}

	lines := strings.Split(ansi.Strip(renderEntryContent(entry, 72, true, styles)), "\n")
	headerRow, resolvedRow, investigatingRow := "", "", ""
	for _, line := range lines {
		switch {
		case strings.Contains(line, "WHEN") && strings.Contains(line, "STATUS") && strings.Contains(line, "DETAILS"):
			headerRow = line
		case strings.Contains(line, "Resolved"):
			resolvedRow = line
		case strings.Contains(line, "Investigating"):
			investigatingRow = line
		}
	}
	require.NotEmpty(t, headerRow)
	require.NotEmpty(t, resolvedRow)
	require.NotEmpty(t, investigatingRow)
	assert.Equal(t, strings.Index(headerRow, "WHEN"), strings.Index(resolvedRow, "Aug"))
	assert.Equal(t, strings.Index(headerRow, "STATUS"), strings.Index(resolvedRow, "Resolved"))
	assert.Equal(t, strings.Index(headerRow, "STATUS"), strings.Index(investigatingRow, "Investigating"))
	assert.Equal(t, strings.Index(headerRow, "DETAILS"), strings.Index(resolvedRow, "Workflows"))
	assert.Equal(t, strings.Index(headerRow, "DETAILS"), strings.Index(investigatingRow, "Engineers"))
}

func TestModelSelectsDashboardEntryThenOpensOverlay(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 120, 40)

	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)
	require.Nil(t, command)
	assert.False(t, model.detailOpen)
	assert.Equal(t, "second", model.detailID)
	assert.Equal(t, 1, model.detailIndex)

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	require.Nil(t, command)
	assert.True(t, model.detailOpen)
	plain := ansi.Strip(model.View().Content)
	assert.Contains(t, plain, "Second incident title in full")
	assert.Contains(t, plain, "Second incident body in full")
	assert.Contains(t, plain, "2 OF 3")
	assert.Contains(t, ansi.Strip(model.view.View()), "GITHUB PULSE")

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(*Model)
	require.Nil(t, command)
	assert.Equal(t, "second", model.detailID)

	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(*Model)
	require.Nil(t, command)
	assert.False(t, model.detailOpen)
	assert.Equal(t, "second", model.detailID)
	assert.Contains(t, ansi.Strip(model.View().Content), "GITHUB PULSE")
}

func TestModelDashboardSelectionScrollsPastVisibleRows(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 80, 24)

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)

	assert.False(t, model.detailOpen)
	assert.Equal(t, "second", model.detailID)
	assert.Equal(t, 1, model.detailIndex)
	plain := ansi.Strip(model.View().Content)
	assert.Contains(t, plain, "Second incident title in full")
	assert.NotContains(t, plain, "First incident title in full")
}

func TestModelStatusHistoryWindowFollowsSelection(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 120, 40)
	model.data.RecentFeed = append(model.data.RecentFeed,
		pulse.FeedEntry{ID: "fourth", Title: "Fourth incident title", UpdatedAt: model.data.GeneratedAt.Add(-3 * time.Minute)},
		pulse.FeedEntry{ID: "fifth", Title: "Fifth incident title", UpdatedAt: model.data.GeneratedAt.Add(-4 * time.Minute)},
	)
	model.syncView()

	for range 3 {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(*Model)
	}

	assert.Equal(t, "fourth", model.detailID)
	assert.Equal(t, 3, model.detailIndex)
	plain := ansi.Strip(model.View().Content)
	assert.NotContains(t, plain, "First incident title in full")
	assert.Contains(t, plain, "Second incident title in full")
	assert.Contains(t, plain, "Third incident title in full")
	assert.Contains(t, plain, "Fourth incident title")
	assert.NotContains(t, plain, "Fifth incident title")
}

func TestModelHighlightsSelectedDashboardEntry(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 120, 40)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)

	var selectedLine string
	for _, line := range strings.Split(ansi.Strip(model.View().Content), "\n") {
		if strings.Contains(line, "Second incident title in full") {
			selectedLine = line
			break
		}
	}
	require.NotEmpty(t, selectedLine)
	assert.Contains(t, selectedLine, "›")
}

func TestModelScrollsOverlayWithoutMovingDashboard(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 80, 24)
	var content strings.Builder
	for index := range 40 {
		fmt.Fprintf(&content, "<p>Update %02d</p>", index)
	}
	model.data.RecentFeed[0].ContentHTML = content.String()
	model.syncView()
	dashboard := model.view.View()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	before := model.View().Content
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)
	after := model.View().Content

	assert.Equal(t, dashboard, model.view.View())
	assert.NotEqual(t, before, after)
}

func TestModelKeepsOverlayChromePinnedWhilePagingToFinalUpdate(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 80, 24)
	var content strings.Builder
	for index := range 40 {
		fmt.Fprintf(&content, "<p>Update %02d</p>", index)
	}
	content.WriteString("<p>FINAL UPDATE MARKER</p>")
	model.data.RecentFeed[0].ContentHTML = content.String()
	model.syncView()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	beforeLines := strings.Split(ansi.Strip(model.View().Content), "\n")
	headerRow, footerRow := -1, -1
	for row, line := range beforeLines {
		if strings.Contains(line, "STATUS HISTORY") && strings.Contains(line, " OF ") {
			headerRow = row
		}
		if strings.Contains(line, "Esc") && strings.Contains(line, "close") {
			footerRow = row
		}
	}
	require.NotEqual(t, -1, headerRow)
	require.NotEqual(t, -1, footerRow)

	for range 20 {
		if model.detailView.AtBottom() {
			break
		}
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
		model = updated.(*Model)
	}
	require.True(t, model.detailView.AtBottom())

	after := ansi.Strip(model.View().Content)
	assert.Contains(t, after, "FINAL UPDATE MARKER")
	afterLines := strings.Split(after, "\n")
	assert.Contains(t, afterLines[headerRow], "STATUS HISTORY")
	assert.Contains(t, afterLines[footerRow], "Esc")
}

func TestModelSeparatesGlobalAndViewerShortcuts(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 200, 50)
	model.data.RecentFeed[0].ContentHTML = strings.Repeat("<p>Long update</p>", 40)
	model.syncView()

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	viewerFooter, globalFooter := "", ""
	viewerFooterRow, globalFooterRow := -1, -1
	for row, line := range lines {
		if strings.Contains(line, "Esc") && strings.Contains(line, "close") {
			viewerFooter = line
			viewerFooterRow = row
		}
		if strings.Contains(line, "q") && strings.Contains(line, "quit") && !strings.Contains(line, "Esc") {
			globalFooter = line
			globalFooterRow = row
		}
	}
	require.NotEmpty(t, viewerFooter)
	require.NotEmpty(t, globalFooter)
	assert.Contains(t, viewerFooter, "scroll")
	assert.Contains(t, viewerFooter, "page")
	assert.NotContains(t, viewerFooter, "quit")
	assert.NotContains(t, viewerFooter, "refresh")
	assert.Contains(t, globalFooter, "quit")
	assert.Contains(t, globalFooter, "refresh")
	assert.NotContains(t, globalFooter, "select")
	assert.NotContains(t, globalFooter, "open")
	assert.NotContains(t, globalFooter, "scroll")
	assert.Equal(t, viewerFooterRow+2, globalFooterRow)
}

func TestModelRefreshKeepsOpenEntryByID(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 120, 40)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	model.data.RecentFeed[1].ContentHTML = strings.Repeat("<p>Original second body</p>", 60)
	model.syncView()
	model.detailView.SetYOffset(5)
	require.Equal(t, 5, model.detailView.YOffset())
	model.fullGen = 1

	next := model.data
	next.RecentFeed = []pulse.FeedEntry{
		{ID: "new", Title: "New incident", ContentHTML: "<p>New body</p>", UpdatedAt: next.GeneratedAt.Add(time.Minute)},
		{ID: "first", Title: "First incident title in full", ContentHTML: "<p>First incident body in full</p>", UpdatedAt: next.GeneratedAt.Add(-time.Minute)},
		{ID: "second", Title: "Second incident title in full", ContentHTML: strings.Repeat("<p>Updated second body</p>", 60), UpdatedAt: next.GeneratedAt.Add(-2 * time.Minute)},
	}
	updated, _ = model.Update(fullMsg{generation: 1, value: next})
	model = updated.(*Model)

	assert.Equal(t, "second", model.detailID)
	entry, index, total, ok := model.detailEntry()
	require.True(t, ok)
	assert.Contains(t, entry.ContentHTML, "Updated second body")
	assert.Equal(t, 2, index)
	assert.Equal(t, 3, total)
	assert.Equal(t, 5, model.detailView.YOffset())
}

func TestModelRefreshFallsBackToNewestDisplayedEntryWhenSelectionDisappears(t *testing.T) {
	t.Parallel()
	model := detailModel(t, 120, 40)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(*Model)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(*Model)
	model.data.RecentFeed[1].ContentHTML = strings.Repeat("<p>Long body line</p>", 60)
	model.syncView()
	model.detailView.SetYOffset(5)
	require.Positive(t, model.detailView.YOffset())
	model.fullGen = 1

	next := model.data
	next.RecentFeed = []pulse.FeedEntry{
		{ID: "new", Title: "Newest incident", ContentHTML: "<p>Newest body</p>" + strings.Repeat("<p>Long body line</p>", 60), UpdatedAt: next.GeneratedAt.Add(time.Minute)},
		{ID: "first", Title: "First incident", ContentHTML: "<p>First body</p>", UpdatedAt: next.GeneratedAt},
	}
	updated, _ = model.Update(fullMsg{generation: 1, value: next})
	model = updated.(*Model)

	assert.Equal(t, "new", model.detailID)
	plain := ansi.Strip(model.View().Content)
	assert.Contains(t, plain, "Newest body")
	assert.Contains(t, plain, "1 OF 2")
	assert.Zero(t, model.detailView.YOffset())
}

func TestRenderEntryDetailFitsSupportedLayoutsAndMonochrome(t *testing.T) {
	t.Parallel()
	longEntry := pulse.FeedEntry{
		ID: "example", Title: strings.Repeat("A long incident title ", 10),
		ContentHTML: `<p>` + strings.Repeat("A long incident paragraph with status information. ", 30) + `</p>`,
		UpdatedAt:   time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC),
	}
	shortEntry := longEntry
	shortEntry.Title = "Short incident"
	shortEntry.ContentHTML = "<p>Short update</p>"

	for _, size := range []struct{ width, height, overlayWidth, overlayHeight int }{
		{80, 24, 76, 23},
		{120, 40, 116, 30},
		{200, 50, 140, 30},
	} {
		shortOutput := renderEntryDetail(shortEntry, 0, 3, detailRenderOptions{width: size.width, height: size.height, mono: true})
		longOutput := renderEntryDetail(longEntry, 0, 3, detailRenderOptions{width: size.width, height: size.height, mono: true})
		assert.NotContains(t, longOutput, "\x1b[38")
		assert.Equal(t, size.overlayWidth, lipgloss.Width(shortOutput))
		assert.Equal(t, size.overlayHeight, lipgloss.Height(shortOutput))
		assert.Equal(t, lipgloss.Width(shortOutput), lipgloss.Width(longOutput))
		assert.Equal(t, lipgloss.Height(shortOutput), lipgloss.Height(longOutput))
	}
}

func detailModel(t *testing.T, width, height int) *Model {
	t.Helper()
	data := fixtureSnapshot(t)
	data.GeneratedAt = time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	data.RecentFeed = []pulse.FeedEntry{
		{ID: "first", Title: "First incident title in full", ContentHTML: "<p>First incident body in full</p>", UpdatedAt: data.GeneratedAt},
		{ID: "second", Title: "Second incident title in full", ContentHTML: "<p>Second incident body in full</p>", UpdatedAt: data.GeneratedAt.Add(-time.Minute)},
		{ID: "third", Title: "Third incident title in full", ContentHTML: "<p>Third incident body in full</p>", UpdatedAt: data.GeneratedAt.Add(-2 * time.Minute)},
	}
	model := New(fetcherStub{data}, true)
	model.ready = true
	model.data = data
	model.width, model.height = width, height
	model.nextRefresh = data.GeneratedAt.Add(refreshEvery)
	model.now = func() time.Time { return data.GeneratedAt }
	model.syncView()
	return model
}
