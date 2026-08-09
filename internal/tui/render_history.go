package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

func renderAggregate(data pulse.Snapshot, width int, s styles, countdown time.Duration) string {
	innerWidth := panelContentWidth(s, width)
	state := data.Overall.State
	if state == "" {
		state = pulse.Unknown
	}
	headerLeft := s.title.Render("GITHUB PULSE")
	if state != pulse.Operational {
		headerLeft += "  " + s.state(state).Bold(true).Render(strings.ToUpper(stateDescription(state)))
	}
	compact := innerWidth < 100
	if data.Sources.History.Available {
		headerLeft += "  " + renderHeaderMetrics(data.History, state, compact, s)
	}
	refresh := "↻ " + formatCountdown(countdown)
	if data.Overall.UpdatedAt != nil {
		if compact {
			refresh = s.timestamp(*data.Overall.UpdatedAt, "15:04 MST") + "  ·  " + refresh
		} else {
			refresh = "UPDATED " + s.timestamp(*data.Overall.UpdatedAt, "15:04 MST") + "  ·  " + refresh
		}
	}
	header := between(headerLeft, s.muted.Render(refresh), innerWidth)
	if !data.Sources.History.Available {
		return s.panel(width).Render(header + "\n\n" + s.muted.Render("Reconstructed history unavailable"))
	}
	return s.panel(width).Render(header + "\n" + renderCombinedHistory(data.History, innerWidth, s))
}

func renderHeaderMetrics(history pulse.History, state pulse.State, compact bool, s styles) string {
	uptime, tracked := "--", "--"
	if history.Uptime90Days != nil {
		uptime = fmt.Sprintf("%.2f%%", *history.Uptime90Days)
	}
	if history.TrackedUptime != nil {
		tracked = fmt.Sprintf("%.2f%%", *history.TrackedUptime)
	}
	coverage := "HISTORY"
	if !history.CoverageStart.IsZero() {
		coverage = fmt.Sprintf("SINCE %d", history.CoverageStart.Year())
		if compact {
			coverage = "SINCE '" + history.CoverageStart.Format("06")
		}
	}
	label := "90D UPTIME"
	if compact {
		label = "90D"
	}
	return s.heroMetric(state).Render(uptime) + " " + s.title.Render(label) + "  ·  " + s.title.Render(tracked) + " " + s.muted.Render(coverage)
}

func renderCombinedHistory(history pulse.History, width int, s styles) string {
	const timelineWidth = 66
	const summaryGap = 2
	const summaryMinWidth = 32
	if width >= timelineWidth+summaryGap+summaryMinWidth {
		detailsWidth := width - timelineWidth - summaryGap
		anchors := between(s.muted.Render("-90 DAYS"), s.muted.Render(latestDayLabel(history)), timelineWidth)
		chartHeaders := lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Width(timelineWidth).Render(anchors),
			strings.Repeat(" ", summaryGap),
			lipgloss.NewStyle().Width(detailsWidth).Render(renderRollingHeader(history, detailsWidth, s)),
		)
		charts := lipgloss.JoinHorizontal(
			lipgloss.Top,
			renderHistoryStrip(history.Days90, timelineWidth, s),
			strings.Repeat(" ", summaryGap),
			renderSparkline(history.Rolling90Days.Series, detailsWidth, s),
		)
		support := lipgloss.JoinHorizontal(
			lipgloss.Top,
			lipgloss.NewStyle().Width(timelineWidth).Render(renderHistoryLegend(s)),
			strings.Repeat(" ", summaryGap),
			lipgloss.NewStyle().Width(detailsWidth).Render(renderRollingDetails(history, detailsWidth, s)),
		)
		divider := s.muted.Render(strings.Repeat("─", width))
		return divider + "\n" + chartHeaders + "\n" + charts + "\n" + support
	}
	divider := s.muted.Render(strings.Repeat("─", width))
	rollingHeader := renderRollingHeader(history, width, s)
	return render90DayTimeline(history, width, s) + "\n" + divider + "\n" + rollingHeader + "\n" + renderSparkline(history.Rolling90Days.Series, width, s) + "\n" + renderRollingDetails(history, width, s)
}

func render90DayTimeline(history pulse.History, width int, s styles) string {
	anchors := between(s.muted.Render("-90 DAYS"), s.muted.Render(latestDayLabel(history)), width)
	return anchors + "\n" + renderHistoryStrip(history.Days90, width, s) + "\n" + renderHistoryLegend(s)
}

func latestDayLabel(history pulse.History) string {
	if len(history.Days90) == 0 {
		return "LATEST COMPLETE DAY"
	}
	return formatRollingDate(history.Days90[len(history.Days90)-1].Date)
}

func renderHistoryLegend(s styles) string {
	return strings.Join([]string{
		legendItem(pulse.Operational, "operational", s),
		legendItem(pulse.Maintenance, "maintenance", s),
		legendItem(pulse.Minor, "minor", s),
		legendItem(pulse.Major, "major", s),
		legendItem(pulse.Critical, "critical", s),
	}, "  ")
}

func renderHistoryStrip(days []pulse.Day, width int, s styles) string {
	if width <= 0 {
		return ""
	}
	if len(days) == 0 {
		return strings.Repeat(stateBar(pulse.Unknown, s), width)
	}
	columns := min(width, len(days))
	var output strings.Builder
	for column := 0; column < columns; column++ {
		start := column * len(days) / columns
		end := (column + 1) * len(days) / columns
		state := pulse.Operational
		for _, day := range days[start:end] {
			state = pulse.WorstState(state, day.State)
		}
		output.WriteString(stateBar(state, s))
	}
	return output.String()
}

func stateBar(state pulse.State, s styles) string {
	style := s.state(state).Bold(false)
	if s.mono {
		switch state {
		case pulse.Operational:
			style = style.Faint(true)
		case pulse.Maintenance:
			style = style.Underline(true)
		case pulse.Major:
			style = style.Bold(true)
		case pulse.Critical:
			style = style.Reverse(true)
		case pulse.Unknown:
			style = style.Underline(true).Reverse(true)
		}
	}
	return style.Render("▮")
}

func legendItem(state pulse.State, label string, s styles) string {
	return stateBar(state, s) + " " + s.muted.Render(label)
}

func renderRollingDetails(history pulse.History, width int, s styles) string {
	left, right := rollingExtremaLabels(history.Rolling90Days, width)
	left = s.state(pulse.Operational).Render(left)
	right = s.state(pulse.Critical).Render(right)
	return between(left, right, width)
}

func renderRollingHeader(history pulse.History, width int, s styles) string {
	downtime := "⯆ -- DAYS"
	if history.Downtime90Days != nil {
		downtime = fmt.Sprintf("⯆ ~%.0f DAYS", math.Round(*history.Downtime90Days))
	}
	return between(s.muted.Render("90-DAY ROLLING"), s.muted.Render(downtime), width)
}

func rollingExtremaLabels(history pulse.RollingHistory, width int) (string, string) {
	left, right := "⯅ --", "⯆ --"
	switch {
	case width >= 43:
		if history.Best != nil {
			left = fmt.Sprintf("⯅ %.2f%% · %s", history.Best.Uptime, formatRollingDate(history.Best.Date))
		}
		if history.Worst != nil {
			right = fmt.Sprintf("⯆ %.2f%% · %s", history.Worst.Uptime, formatRollingDate(history.Worst.Date))
		}
	case width >= 24:
		if history.Best != nil {
			left = fmt.Sprintf("⯅ %.2f%%", history.Best.Uptime)
		}
		if history.Worst != nil {
			right = fmt.Sprintf("⯆ %.2f%%", history.Worst.Uptime)
		}
	default:
		if history.Best != nil {
			left = fmt.Sprintf("⯅%.1f%%", history.Best.Uptime)
		}
		if history.Worst != nil {
			right = fmt.Sprintf("⯆%.1f%%", history.Worst.Uptime)
		}
	}
	return left, right
}

func formatRollingDate(value string) string {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return value
	}
	return formatDate(date)
}

func renderSparkline(points []pulse.RollingPoint, width int, s styles) string {
	if len(points) == 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	minValue, maxValue := points[0].Uptime, points[0].Uptime
	for _, point := range points {
		minValue = math.Min(minValue, point.Uptime)
		maxValue = math.Max(maxValue, point.Uptime)
	}
	var output strings.Builder
	for column := 0; column < min(width, len(points)); column++ {
		index := column * (len(points) - 1) / max(1, min(width, len(points))-1)
		var level int
		if maxValue > minValue {
			level = int(math.Round((points[index].Uptime - minValue) / (maxValue - minValue) * float64(len(blocks)-1)))
		} else {
			level = int(math.Round(points[index].Uptime / 100 * float64(len(blocks)-1)))
		}
		level = min(max(level, 0), len(blocks)-1)
		bar := string(blocks[level])
		if !s.mono {
			bar = lipgloss.NewStyle().Foreground(lipgloss.Color(sparklineColor(level))).Render(bar)
		}
		output.WriteString(bar)
	}
	return output.String()
}

var sparklineGradient = []string{
	"#f04444",
	"#f0883e",
	"#d8a52b",
	"#d8a52b",
	"#b6b53d",
	"#8fc14b",
	"#68c75a",
	"#47c96b",
}

func sparklineColor(level int) string {
	return sparklineGradient[min(max(level, 0), len(sparklineGradient)-1)]
}
