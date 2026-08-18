// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

type renderOptions struct {
	width, height int
	mono          bool
	scrollable    bool
	selectedFeed  int
	feedOffset    int
	countdown     time.Duration
	now           time.Time
	detailOpen    bool
}

var osc8SequencePattern = regexp.MustCompile(`\x1b\]8;[^;\x07]*;[^\x07]*\x07`)

func render(data pulse.Snapshot, options renderOptions) string {
	width := min(max(options.width-4, 36), 140)
	s := newStyles(options.mono)
	sections := []string{
		renderAggregate(data, width, s, options.countdown),
	}
	if context := renderContext(data, width, options.now, s); context != "" {
		sections = append(sections, context)
	}
	sections = append(sections, renderComponents(data.Components, data.History.Components, width, s), renderFeed(data, width, options.height, options.selectedFeed, options.feedOffset, s))
	if issues := renderSourceIssues(data.Errors, width, s); issues != "" {
		sections = append(sections, issues)
	}
	if !options.detailOpen {
		canViewDetails := data.Sources.Feed.Available && len(data.RecentFeed) > 0
		sections = append(sections, renderFooter(width, s, options.scrollable, canViewDetails))
	}
	return lipgloss.PlaceHorizontal(options.width, lipgloss.Center, strings.Join(sections, "\n"))
}

func renderContext(data pulse.Snapshot, width int, now time.Time, s styles) string {
	innerWidth := panelContentWidth(s, width)
	lines := make([]string, 0, len(data.ActiveIncidents)+len(data.ActiveMaintenances))
	for _, incident := range data.ActiveIncidents {
		lines = append(lines, renderIncident(incident, innerWidth, now, s))
	}
	for _, maintenance := range data.ActiveMaintenances {
		line := s.status(pulse.Maintenance) + "  " + maintenance.Name + "  ·  " + strings.ToUpper(maintenance.Status)
		if bounds := maintenanceBounds(maintenance, s); bounds != "" {
			line += "  ·  " + bounds
		}
		lines = append(lines, truncate(line, innerWidth))
	}
	if len(lines) == 0 {
		return ""
	}
	body := s.title.Render("ACTIVE CONTEXT") + "\n\n" + strings.Join(lines, "\n")
	return s.panel(width).Render(body)
}

func renderIncident(incident pulse.Incident, width int, now time.Time, s styles) string {
	prefix := s.status(incident.State) + "  "
	suffix := "  ·  " + strings.ToUpper(incident.Status)
	if incident.StartedAt != nil {
		suffix += "  ·  " + s.incidentAge(now.Sub(*incident.StartedAt))
	}

	nameWidth := width - ansi.StringWidth(prefix) - ansi.StringWidth(suffix)
	core := prefix + suffix
	if nameWidth > 0 {
		core = prefix + truncate(incident.Name, nameWidth) + suffix
	}
	core = truncate(core, width)
	if incident.LatestUpdate == nil || incident.LatestUpdate.Body == "" {
		return core
	}

	updatePrefix := "  " + s.muted.Render("— ")
	updateWidth := width - ansi.StringWidth(core) - ansi.StringWidth(updatePrefix)
	if updateWidth <= 0 {
		return core
	}
	return core + updatePrefix + s.muted.Render(truncate(incident.LatestUpdate.Body, updateWidth))
}

func maintenanceBounds(value pulse.MaintenanceWindow, s styles) string {
	switch {
	case value.ScheduledFor != nil && value.ScheduledUntil != nil:
		start := value.ScheduledFor.In(s.location)
		end := value.ScheduledUntil.In(s.location)
		_, startOffset := start.Zone()
		_, endOffset := end.Zone()
		layout := "15:04 MST"
		if start.Year() != end.Year() || start.YearDay() != end.YearDay() || startOffset != endOffset {
			layout = "2006-01-02 15:04 MST"
		}
		return start.Format(layout) + "–" + end.Format(layout)
	case value.ScheduledFor != nil:
		return "FROM " + s.timestamp(*value.ScheduledFor, "15:04 MST")
	case value.ScheduledUntil != nil:
		return "UNTIL " + s.timestamp(*value.ScheduledUntil, "15:04 MST")
	default:
		return ""
	}
}

func renderFeed(data pulse.Snapshot, width, terminalHeight, selected, offset int, s styles) string {
	innerWidth := panelContentWidth(s, width)
	header := s.title.Render("STATUS HISTORY")
	if !data.Sources.Feed.Available {
		return s.panel(width).Render(header + "\n\n" + s.muted.Render("Feed unavailable"))
	}
	limit := feedEntryLimit(innerWidth, terminalHeight)
	start, end := feedWindow(len(data.RecentFeed), selected, offset, limit)
	if start == end {
		return s.panel(width).Render(header + "\n\n" + s.muted.Render("No recent entries"))
	}

	scrollable := len(data.RecentFeed) > end-start
	contentWidth := innerWidth
	if scrollable {
		contentWidth = max(1, contentWidth-2)
	}
	lines := make([]string, 0, end-start)
	for index, entry := range data.RecentFeed[start:end] {
		stampText := s.timestamp(entry.UpdatedAt, "2006-01-02 15:04 MST")
		stamp := s.muted.Render(stampText)
		title := truncate(stripUnsafeTerminalLine(entry.Title), max(1, contentWidth-ansi.StringWidth(stampText)-4))
		if entry.URL != nil {
			title = terminalLink(title, *entry.URL)
		}
		line := "  " + stamp + "  " + title
		if start+index == selected {
			line = s.selected.Render("› " + stampText + "  " + title)
		}
		lines = append(lines, lipgloss.NewStyle().Width(contentWidth).Render(line))
	}
	history := strings.Join(lines, "\n")
	if scrollable {
		scrollbar := renderVerticalScrollbar(len(lines), len(data.RecentFeed), len(lines), start, s)
		history = lipgloss.JoinHorizontal(lipgloss.Top, history, " ", scrollbar)
	}
	return s.panel(width).Render(header + "\n\n" + history)
}

func renderVerticalScrollbar(height, total, visible, offset int, s styles) string {
	if height <= 0 || total <= visible {
		return ""
	}
	visible = min(max(visible, 1), total)
	thumbHeight := min(height, max(1, (height*visible+total-1)/total))
	maxOffset := total - visible
	offset = min(max(offset, 0), maxOffset)
	thumbStart := 0
	if available := height - thumbHeight; available > 0 {
		thumbStart = (offset*available + maxOffset/2) / maxOffset
	}
	lines := make([]string, height)
	for row := range height {
		if row >= thumbStart && row < thumbStart+thumbHeight {
			lines[row] = s.title.Render("┃")
		} else {
			lines[row] = s.muted.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

func feedWindow(total, selected, offset, limit int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	selected = min(max(selected, 0), total-1)
	limit = min(max(limit, 1), total)
	offset = min(max(offset, 0), total-limit)
	if selected < offset {
		offset = selected
	}
	if selected >= offset+limit {
		offset = selected - limit + 1
	}
	return offset, min(total, offset+limit)
}

func fitTableCell(value string, width int) string {
	value = truncate(value, width)
	return value + strings.Repeat(" ", max(0, width-ansi.StringWidth(value)))
}

func feedEntryLimit(innerWidth, terminalHeight int) int {
	if innerWidth < 96 || terminalHeight < 32 {
		return 1
	}
	return 3
}

func terminalLink(label, target string) string {
	label = stripUnsafeTerminalLine(label)
	if !safeHyperlinkTarget(target) {
		return label
	}
	return ansi.SetHyperlink(target) + label + ansi.ResetHyperlink()
}

func safeHyperlinkTarget(target string) bool {
	if stripUnsafeTerminalLine(target) != target {
		return false
	}
	parsed, err := url.ParseRequestURI(target)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func filterTerminalHyperlinks(value string) string {
	return osc8SequencePattern.ReplaceAllStringFunc(value, func(sequence string) string {
		payload := strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b]8;"), "\x07")
		_, target, _ := strings.Cut(payload, ";")
		if target == "" || safeHyperlinkTarget(target) {
			return sequence
		}
		return ""
	})
}

func renderFooter(width int, s styles, scrollable, canViewDetails bool) string {
	innerWidth := panelContentWidth(s, width)
	actions := []string{s.keycap("q") + " quit"}
	if scrollable {
		actions = append(actions, s.keycap("j/k")+" scroll")
	}
	if canViewDetails {
		actions = append(actions, s.keycap("↑/↓")+" select", s.keycap("Enter")+" open")
	}
	actions = append(actions, s.keycap("r")+" refresh")
	keys := actions[0]
	for _, action := range actions[1:] {
		candidate := keys + "  " + action
		if ansi.StringWidth(candidate) > innerWidth {
			break
		}
		keys = candidate
	}
	return truncate(keys, innerWidth)
}

func renderSourceIssues(errors []pulse.SourceError, width int, s styles) string {
	if len(errors) == 0 {
		return ""
	}
	innerWidth := panelContentWidth(s, width)
	lines := make([]string, 0, len(errors))
	for _, sourceError := range errors {
		message := sourceError.Source + ": " + sourceError.Message
		if sourceError.AttemptedAt != nil {
			message += "  ·  attempt " + s.timestamp(*sourceError.AttemptedAt, "15:04 MST")
		}
		lines = append(lines, s.state(pulse.Minor).Render(truncate(message, innerWidth)))
	}
	return s.panel(width).Render(s.title.Render("SOURCE ISSUES") + "\n\n" + strings.Join(lines, "\n"))
}

func formatCountdown(remaining time.Duration) string {
	seconds := max(0, int((remaining+time.Second-1)/time.Second))
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func formatElapsed(elapsed time.Duration) string {
	minutes := max(int64(0), int64(elapsed/time.Minute))
	days := minutes / (24 * 60)
	hours := minutes / 60 % 24
	minutes %= 60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, " ")
}

func formatDate(value time.Time) string {
	return value.Format(time.DateOnly)
}

func panelContentWidth(s styles, width int) int {
	frameWidth, _ := s.panel(width).GetFrameSize()
	return max(1, width-frameWidth)
}

func stateDescription(state pulse.State) string {
	switch state {
	case pulse.Operational:
		return "All systems operational"
	case pulse.Maintenance:
		return "Maintenance in progress"
	case pulse.Minor:
		return "Minor service degradation"
	case pulse.Major:
		return "Major service disruption"
	case pulse.Critical:
		return "Critical service outage"
	default:
		return "Status unavailable"
	}
}

func between(left, right string, width int) string {
	if right == "" {
		return truncate(left, width)
	}
	rightWidth := ansi.StringWidth(right)
	if rightWidth >= width {
		return truncate(right, width)
	}
	left = truncate(left, width-rightWidth-1)
	gap := max(1, width-ansi.StringWidth(left)-rightWidth)
	return left + strings.Repeat(" ", gap) + right
}

func truncate(value string, width int) string {
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return ansi.Truncate(value, width, "…")
}
