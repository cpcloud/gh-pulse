// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"fmt"
	"math/big"
	"slices"
	"sort"
	"time"

	"github.com/cpcloud/gh-pulse/internal/pulse"
)

var CoverageStart = time.Date(2022, 6, 11, 0, 0, 0, 0, time.UTC)

const rollingDays = 90

// supportedComponents limits component output to GitHub's public service names.
// A service is emitted only when mrshu/github-statuses contains an interval
// explicitly tagged with that exact name. Its 90-day result uses only those
// tagged intervals: non-maintenance intervals merge before downtime is summed,
// while maintenance still marks its day. A missing tag therefore remains
// unavailable instead of becoming an assumed 100% value.
var supportedComponents = []string{
	"Git Operations",
	"Webhooks",
	"API Requests",
	"Issues",
	"Pull Requests",
	"Actions",
	"Packages",
	"Pages",
	"Codespaces",
	"Copilot",
	"Copilot AI Model Providers",
}

// Calculate follows the approved reconstructed source's rule: every positive
// non-maintenance interval, including impact "none", is downtime. Maintenance
// is excluded, overlapping intervals merge, and coverage starts 2022-06-11.
func Calculate(intervals []Interval, now time.Time) (pulse.History, error) {
	asOf := utcDay(now)
	if asOf.Before(CoverageStart.AddDate(0, 0, rollingDays)) {
		return pulse.History{}, fmt.Errorf("history does not contain a complete 90-day window")
	}
	for _, interval := range intervals {
		if interval.End.Before(interval.Start) || !validImpact(interval.Impact) {
			return pulse.History{}, fmt.Errorf("invalid history interval")
		}
	}

	days := make([]pulse.Day, 0, rollingDays)
	windowStart := asOf.AddDate(0, 0, -rollingDays)
	for day := windowStart; day.Before(asOf); day = day.AddDate(0, 0, 1) {
		days = append(days, pulse.Day{Date: day.Format(time.DateOnly), State: dailyState(intervals, day)})
	}

	windowDown := downtime(intervals, windowStart, asOf)
	trackedDown := downtime(intervals, CoverageStart, asOf)
	windowUptime := roundPercent(asOf.Sub(windowStart)-windowDown, asOf.Sub(windowStart))
	trackedUptime := roundPercent(asOf.Sub(CoverageStart)-trackedDown, asOf.Sub(CoverageStart))

	series := make([]pulse.RollingPoint, 0, int(asOf.Sub(CoverageStart).Hours()/24)-rollingDays+1)
	var best, worst *pulse.RollingPoint
	var bestDown, worstDown time.Duration
	for boundary := CoverageStart.AddDate(0, 0, rollingDays); !boundary.After(asOf); boundary = boundary.AddDate(0, 0, 1) {
		start := boundary.AddDate(0, 0, -rollingDays)
		total := boundary.Sub(start)
		down := downtime(intervals, start, boundary)
		point := pulse.RollingPoint{Date: boundary.Format(time.DateOnly), Uptime: roundPercent(total-down, total)}
		series = append(series, point)
		if best == nil || down < bestDown {
			copy := point
			best, bestDown = &copy, down
		}
		if worst == nil || down > worstDown {
			copy := point
			worst, worstDown = &copy, down
		}
	}
	current := series[len(series)-1]
	components := make([]pulse.ComponentHistory, 0, len(supportedComponents))
	for _, name := range supportedComponents {
		componentIntervals := filterComponent(intervals, name)
		if len(componentIntervals) == 0 {
			continue
		}
		componentDays := make([]pulse.Day, 0, rollingDays)
		for day := windowStart; day.Before(asOf); day = day.AddDate(0, 0, 1) {
			componentDays = append(componentDays, pulse.Day{Date: day.Format(time.DateOnly), State: dailyState(componentIntervals, day)})
		}
		uptime := roundPercent(asOf.Sub(windowStart)-downtime(componentIntervals, windowStart, asOf), asOf.Sub(windowStart))
		components = append(components, pulse.ComponentHistory{Name: name, Days90: componentDays, Uptime90Days: floatPtr(uptime)})
	}
	return pulse.History{
		Source: "mrshu/github-statuses", CoverageStart: CoverageStart, AsOf: asOf,
		Days90: days, Uptime90Days: floatPtr(windowUptime), Downtime90Days: floatPtr(windowDown.Hours() / 24),
		TrackedUptime: floatPtr(trackedUptime),
		Rolling90Days: pulse.RollingHistory{Current: &current, Best: best, Worst: worst, Series: series},
		Components:    components,
	}, nil
}

func filterComponent(intervals []Interval, name string) []Interval {
	filtered := make([]Interval, 0)
	for _, interval := range intervals {
		if slices.Contains(interval.Components, name) {
			filtered = append(filtered, interval)
		}
	}
	return filtered
}

func utcDay(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func dailyState(intervals []Interval, day time.Time) pulse.State {
	end := day.AddDate(0, 0, 1)
	state := pulse.Operational
	for _, interval := range intervals {
		if !interval.End.After(interval.Start) || !interval.Start.Before(end) || !interval.End.After(day) {
			continue
		}
		candidate := impactState(interval.Impact)
		state = pulse.WorstState(state, candidate)
	}
	return state
}

func impactState(impact Impact) pulse.State {
	switch impact {
	case ImpactMaintenance:
		return pulse.Maintenance
	case ImpactMinor:
		return pulse.Minor
	case ImpactMajor:
		return pulse.Major
	case ImpactCritical:
		return pulse.Critical
	default:
		return pulse.Operational
	}
}

type span struct{ start, end time.Time }

func downtime(intervals []Interval, start, end time.Time) time.Duration {
	spans := make([]span, 0, len(intervals))
	for _, interval := range intervals {
		if interval.Impact == ImpactMaintenance || !interval.End.After(interval.Start) {
			continue
		}
		left, right := interval.Start, interval.End
		if left.Before(start) {
			left = start
		}
		if right.After(end) {
			right = end
		}
		if right.After(left) {
			spans = append(spans, span{left, right})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	var total time.Duration
	for index := 0; index < len(spans); {
		merged := spans[index]
		index++
		for index < len(spans) && !spans[index].start.After(merged.end) {
			if spans[index].end.After(merged.end) {
				merged.end = spans[index].end
			}
			index++
		}
		total += merged.end.Sub(merged.start)
	}
	return total
}

func roundPercent(available, total time.Duration) float64 {
	if total <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(int64(available)), big.NewInt(10000))
	denominator := big.NewInt(int64(total))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(denominator) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return float64(quotient.Int64()) / 100
}

func floatPtr(value float64) *float64 { return &value }
