// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

type detailRenderOptions struct {
	width, height int
	mono          bool
	view          *viewport.Model
}

func renderEntryDetail(entry pulse.FeedEntry, index, total int, options detailRenderOptions) string {
	s := newStyles(options.mono)
	layout := entryDetailLayout(options.width, options.height, s)
	detailView := options.view
	if detailView == nil {
		view := viewport.New()
		configureDetailView(&view, entry, layout, options.mono, s)
		detailView = &view
	}
	scrollable := detailViewScrollable(detailView)
	body := detailView.View()
	if scrollable {
		scrollbar := renderVerticalScrollbar(
			detailView.Height(), detailView.TotalLineCount(), detailView.Height(), detailView.YOffset(), s,
		)
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, " ", scrollbar)
	}
	header := between(s.title.Render("STATUS HISTORY"), s.muted.Render(fmt.Sprintf("%d OF %d", index+1, total)), layout.innerWidth)
	rule := s.muted.Render(strings.Repeat("─", layout.innerWidth))
	controls := renderDetailFooter(layout.innerWidth, s, scrollable, total > 1)
	content := strings.Join([]string{header, rule, body, rule, controls}, "\n")
	return s.panel(layout.width).Height(layout.height).Render(content)
}

func configureDetailView(view *viewport.Model, entry pulse.FeedEntry, layout detailLayout, mono bool, s styles) {
	offset := view.YOffset()
	view.SetHeight(layout.bodyHeight)
	view.FillHeight = true
	view.SetWidth(layout.innerWidth)
	view.SetContent(renderEntryContent(entry, layout.innerWidth, mono, s))
	if detailViewScrollable(view) {
		contentWidth := max(1, layout.innerWidth-2)
		view.SetWidth(contentWidth)
		view.SetContent(renderEntryContent(entry, contentWidth, mono, s))
	}
	view.SetYOffset(offset)
}

func detailViewScrollable(view *viewport.Model) bool {
	return view.TotalLineCount() > view.Height()
}

type detailLayout struct {
	width, height          int
	innerWidth, bodyHeight int
}

func entryDetailLayout(terminalWidth, terminalHeight int, s styles) detailLayout {
	width := min(max(terminalWidth-4, 36), 140)
	height := min(30, max(8, terminalHeight-1))
	frameWidth, frameHeight := s.panel(width).GetFrameSize()
	return detailLayout{
		width: width, height: height,
		innerWidth: max(1, width-frameWidth), bodyHeight: max(1, height-frameHeight-4),
	}
}

func renderEntryContent(entry pulse.FeedEntry, width int, mono bool, s styles) string {
	stamp := s.muted.Render(s.timestamp(entry.UpdatedAt, "2006-01-02 15:04 MST"))
	title := ansi.Wrap(stripUnsafeTerminalLine(entry.Title), width, " ")
	if entry.URL != nil {
		title = terminalMultilineLink(title, *entry.URL)
	}
	body := renderEntryBody(entry.ContentHTML, entry.UpdatedAt, width, mono, s)
	return strings.Join([]string{stamp, title, "", body}, "\n")
}

func renderEntryBody(contentHTML string, reference time.Time, width int, mono bool, s styles) string {
	if strings.TrimSpace(contentHTML) == "" {
		return s.muted.Render("No additional details provided")
	}
	if updates, ok := parseEntryUpdates(contentHTML, reference, s.location); ok {
		if table, fits := renderEntryUpdateTable(updates, width, s); fits {
			return table
		}
	}
	markdown, err := entryMarkdown(contentHTML, reference, s.location)
	if err != nil {
		return s.state(pulse.Minor).Render("Entry content unavailable")
	}
	style := glamourstyles.DarkStyleConfig
	if mono {
		style = glamourstyles.NoTTYStyleConfig
		enabled := true
		style.Strong.BlockPrefix = ""
		style.Strong.BlockSuffix = ""
		style.Strong.Bold = &enabled
		style.Heading.Bold = &enabled
		style.H1.Prefix, style.H1.Suffix = "", ""
		style.H2.Prefix, style.H2.Suffix = "", ""
		style.H3.Prefix, style.H3.Suffix = "", ""
		style.H4.Prefix, style.H4.Suffix = "", ""
		style.H5.Prefix, style.H5.Suffix = "", ""
		style.H6.Prefix, style.H6.Suffix = "", ""
	}
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	style.HorizontalRule.Format = s.muted.Render(strings.Repeat("─", width))
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return s.state(pulse.Minor).Render("Entry content unavailable")
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return s.state(pulse.Minor).Render("Entry content unavailable")
	}
	return filterTerminalHyperlinks(strings.TrimSpace(rendered))
}

func renderEntryUpdateTable(updates []entryUpdate, width int, s styles) (string, bool) {
	const timestampLayout = "Jan 02, 15:04 MST"

	whenWidth := ansi.StringWidth("WHEN")
	statusWidth := ansi.StringWidth("STATUS")
	for _, update := range updates {
		whenWidth = max(whenWidth, ansi.StringWidth(s.timestamp(update.when, timestampLayout)))
		statusWidth = max(statusWidth, ansi.StringWidth(update.status))
	}
	statusWidth = min(statusWidth, 16)
	detailsWidth := width - whenWidth - statusWidth - 4
	if detailsWidth < 12 {
		return "", false
	}

	header := strings.Join([]string{
		s.muted.Render(fitTableCell("WHEN", whenWidth)),
		s.muted.Render(fitTableCell("STATUS", statusWidth)),
		s.muted.Render("DETAILS"),
	}, "  ")
	rule := s.muted.Render(strings.Repeat("─", width))
	lines := []string{header, rule}
	for index, update := range updates {
		detailLines := strings.Split(ansi.Wrap(update.details, detailsWidth, " "), "\n")
		for lineIndex, detailLine := range detailLines {
			when, status := "", ""
			if lineIndex == 0 {
				when = s.muted.Render(fitTableCell(s.timestamp(update.when, timestampLayout), whenWidth))
				status = entryStatusStyle(update.status, s).Render(fitTableCell(update.status, statusWidth))
			} else {
				when = strings.Repeat(" ", whenWidth)
				status = strings.Repeat(" ", statusWidth)
			}
			lines = append(lines, when+"  "+status+"  "+detailLine)
		}
		if index < len(updates)-1 {
			lines = append(lines, rule)
		}
	}
	return strings.Join(lines, "\n"), true
}

func entryStatusStyle(status string, s styles) lipgloss.Style {
	state := pulse.Unknown
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "resolved":
		state = pulse.Operational
	case "monitoring":
		state = pulse.Maintenance
	case "update":
		state = pulse.Minor
	case "identified":
		state = pulse.Major
	case "investigating":
		state = pulse.Critical
	}
	return s.state(state).Bold(true)
}

func renderDetailFooter(width int, s styles, scrollable, pageable bool) string {
	actions := []string{s.keycap("Esc") + " close"}
	if pageable {
		actions = append(actions, s.keycap("←/→")+" entry")
	}
	if scrollable {
		actions = append(actions, s.keycap("↑/↓")+" scroll")
		actions = append(actions, s.keycap("PgUp/PgDn")+" page")
	}
	keys := actions[0]
	for _, action := range actions[1:] {
		candidate := keys + "  " + action
		if ansi.StringWidth(candidate) > width {
			break
		}
		keys = candidate
	}
	return truncate(keys, width)
}
