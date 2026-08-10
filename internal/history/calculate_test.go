// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"testing"
	"time"

	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateMergesDowntimeExcludesMaintenanceAndHonorsDailySeverity(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2024, 3, 3, 17, 0, 0, 0, time.FixedZone("local", 3600))
	intervals := []Interval{
		{Start: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), End: time.Date(2024, 2, 29, 2, 0, 0, 0, time.UTC), Impact: ImpactMaintenance},
		{Start: time.Date(2024, 2, 29, 1, 0, 0, 0, time.UTC), End: time.Date(2024, 2, 29, 3, 0, 0, 0, time.UTC), Impact: ImpactMinor},
		{Start: time.Date(2024, 2, 29, 2, 0, 0, 0, time.UTC), End: time.Date(2024, 2, 29, 4, 0, 0, 0, time.UTC), Impact: ImpactMajor},
		{Start: time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC), Impact: ImpactCritical},
		{Start: time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC), End: time.Date(2024, 3, 2, 1, 0, 0, 0, time.UTC), Impact: ImpactNone},
	}

	got, err := Calculate(intervals, asOf)
	require.NoError(t, err)
	assert.Equal(t, "2024-03-03", got.AsOf.Format(time.DateOnly))
	require.Len(t, got.Days90, 90)
	states := map[string]pulse.State{}
	for _, day := range got.Days90 {
		states[day.Date] = day.State
	}
	assert.Equal(t, pulse.Major, states["2024-02-29"])
	assert.Equal(t, pulse.Operational, states["2024-03-01"])
	assert.Equal(t, pulse.Operational, states["2024-03-02"])
	require.NotNil(t, got.Uptime90Days)
	require.NotNil(t, got.Downtime90Days)
	assert.InDelta(t, 99.81, *got.Uptime90Days, 0.000001)
	assert.InDelta(t, float64(4)/24, *got.Downtime90Days, 0.000001)
}

func TestCalculateSelectsDistinctRollingExtremaAndTrackedUptime(t *testing.T) {
	t.Parallel()
	asOf := CoverageStart.AddDate(0, 0, 92)
	intervals := []Interval{{
		Start:  CoverageStart.Add(12 * time.Hour),
		End:    CoverageStart.Add(24 * time.Hour),
		Impact: ImpactMajor,
	}}

	got, err := Calculate(intervals, asOf)
	require.NoError(t, err)
	require.NotNil(t, got.Rolling90Days.Best)
	require.NotNil(t, got.Rolling90Days.Worst)
	require.NotNil(t, got.TrackedUptime)
	assert.Equal(t, CoverageStart.AddDate(0, 0, 91).Format(time.DateOnly), got.Rolling90Days.Best.Date)
	assert.Equal(t, CoverageStart.AddDate(0, 0, 90).Format(time.DateOnly), got.Rolling90Days.Worst.Date)
	assert.Greater(t, got.Rolling90Days.Best.Uptime, got.Rolling90Days.Worst.Uptime)
	assert.InDelta(t, 99.46, *got.TrackedUptime, 0.000001)
}

func TestCalculateStartsRollingOnlyAfterCompleteWindowAndUsesEarliestTie(t *testing.T) {
	t.Parallel()
	asOf := CoverageStart.AddDate(0, 0, 92)
	got, err := Calculate(nil, asOf)
	require.NoError(t, err)
	require.Len(t, got.Rolling90Days.Series, 3)
	first := got.Rolling90Days.Series[0]
	assert.Equal(t, CoverageStart.AddDate(0, 0, 90).Format(time.DateOnly), first.Date)
	require.NotNil(t, got.Rolling90Days.Best)
	require.NotNil(t, got.Rolling90Days.Worst)
	assert.Equal(t, first.Date, got.Rolling90Days.Best.Date)
	assert.Equal(t, first.Date, got.Rolling90Days.Worst.Date)
}

func TestCalculateUsesHalfOpenMidnightAcrossLeapDay(t *testing.T) {
	t.Parallel()
	intervals := []Interval{{
		Start: time.Date(2024, 2, 28, 23, 0, 0, 0, time.UTC),
		End:   time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), Impact: ImpactMinor,
	}}
	got, err := Calculate(intervals, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	states := map[string]pulse.State{}
	for _, day := range got.Days90 {
		states[day.Date] = day.State
	}
	assert.Equal(t, pulse.Minor, states["2024-02-28"])
	assert.Equal(t, pulse.Operational, states["2024-02-29"])
}

func TestRoundPercentHandlesMultiYearDurationsAndHalfTies(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 99.99, roundPercent(99985, 100000), 0.000001)
	fiveYears := 5 * 365 * 24 * time.Hour
	assert.InDelta(t, 100, roundPercent(fiveYears-time.Hour, fiveYears), 0.000001)
}

func TestCalculateRejectsRangeBeforeCompleteCoverage(t *testing.T) {
	t.Parallel()
	_, err := Calculate(nil, CoverageStart.AddDate(0, 0, 89))
	require.Error(t, err)
}

func TestCalculateBuildsIndependentComponentHistory(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	start := asOf.AddDate(0, 0, -1)
	intervals := []Interval{
		{Start: start, End: start.Add(2 * time.Hour), Impact: ImpactMinor, Components: []string{"Git Operations"}},
		{Start: start.Add(time.Hour), End: start.Add(3 * time.Hour), Impact: ImpactMajor, Components: []string{"Git Operations"}},
		{Start: start, End: start.Add(8 * time.Hour), Impact: ImpactMaintenance, Components: []string{"API Requests"}},
	}

	got, err := Calculate(intervals, asOf)
	require.NoError(t, err)
	require.Len(t, got.Components, 2)

	byName := make(map[string]pulse.ComponentHistory, len(got.Components))
	for _, component := range got.Components {
		byName[component.Name] = component
	}
	git := byName["Git Operations"]
	require.NotNil(t, git.Uptime90Days)
	assert.InDelta(t, 99.86, *git.Uptime90Days, 0.000001)
	assert.Equal(t, pulse.Major, git.Days90[len(git.Days90)-1].State)
	api := byName["API Requests"]
	require.NotNil(t, api.Uptime90Days)
	assert.InDelta(t, 100, *api.Uptime90Days, 0.000001)
	assert.Equal(t, pulse.Maintenance, api.Days90[len(api.Days90)-1].State)
	_, hasIssues := byName["Issues"]
	assert.False(t, hasIssues)
}

func TestCalculateIncludesCopilotModelProviderHistory(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)

	got, err := Calculate([]Interval{{
		Start: start, End: start.Add(time.Hour), Impact: ImpactMinor,
		Components: []string{"Copilot AI Model Providers"},
	}}, now)
	require.NoError(t, err)

	byName := make(map[string]pulse.ComponentHistory, len(got.Components))
	for _, component := range got.Components {
		byName[component.Name] = component
	}
	providers, ok := byName["Copilot AI Model Providers"]
	require.True(t, ok)
	require.NotNil(t, providers.Uptime90Days)
	assert.Less(t, *providers.Uptime90Days, 100.0)
	assert.Contains(t, providers.Days90, pulse.Day{Date: start.Format(time.DateOnly), State: pulse.Minor})
}
