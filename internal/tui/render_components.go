package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

func renderComponents(components []pulse.Component, histories []pulse.ComponentHistory, width int, s styles) string {
	innerWidth := panelContentWidth(s, width)
	if len(components) == 0 {
		return s.panel(width).Render(s.title.Render("COMPONENTS") + "\n\n" + s.muted.Render("No component data"))
	}
	groups := make(map[string]bool)
	for _, component := range components {
		if component.Group {
			groups[component.ID] = true
		}
	}

	sections := make([]string, 0)
	ungrouped := make([]pulse.Component, 0)
	grouped := make(map[string][]pulse.Component)
	for _, component := range components {
		if component.Group {
			continue
		}
		if component.GroupID != nil && groups[*component.GroupID] {
			grouped[*component.GroupID] = append(grouped[*component.GroupID], component)
			continue
		}
		ungrouped = append(ungrouped, component)
	}
	if len(ungrouped) > 0 {
		sections = append(sections, renderComponentGrid(ungrouped, histories, innerWidth, s))
	}
	for _, component := range components {
		children := grouped[component.ID]
		if !component.Group || len(children) == 0 {
			continue
		}
		section := s.title.Render(strings.ToUpper(component.Name)) + "\n"
		section += renderComponentGrid(children, histories, innerWidth, s)
		sections = append(sections, section)
	}
	return s.panel(width).Render(s.title.Render("COMPONENTS") + "\n" + strings.Join(sections, "\n\n"))
}

func renderComponentGrid(components []pulse.Component, histories []pulse.ComponentHistory, width int, s styles) string {
	columns := 1
	if width >= 120 {
		columns = 2
	}
	cellWidths := []int{width}
	if columns == 2 {
		leftWidth := (width - 3) / 2
		cellWidths = []int{leftWidth, width - 3 - leftWidth}
	}
	headerCell := renderComponentHeader(cellWidths[0], s)
	gridHeader := headerCell
	if columns == 2 {
		rightHeader := renderComponentHeader(cellWidths[1], s)
		gridHeader = lipgloss.JoinHorizontal(lipgloss.Top, headerCell, s.muted.Render(" │ "), rightHeader)
	}
	rows := []string{gridHeader}
	for index := 0; index < len(components); index += columns {
		cells := make([]string, 0, columns)
		for column := 0; column < columns && index+column < len(components); column++ {
			component := components[index+column]
			cells = append(cells, renderComponent(component, findComponentHistory(histories, component.Name), cellWidths[column], s))
		}
		if len(cells) == 2 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells[0], s.muted.Render(" │ "), cells[1]))
		} else {
			rows = append(rows, cells[0])
		}
	}
	return strings.Join(rows, "\n")
}

func renderComponentHeader(width int, s styles) string {
	columns := componentColumnsFor(width)
	historyLabel := "30D HISTORY"
	if columns.history < ansi.StringWidth(historyLabel) {
		historyLabel = "30D"
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(columns.status).Render(s.muted.Render("NOW")),
		" ",
		lipgloss.NewStyle().Width(columns.component).Render(s.muted.Render("COMPONENT")),
		" ",
		lipgloss.NewStyle().Width(columns.uptime).Align(lipgloss.Right).Render(s.muted.Render("90D UPTIME")),
		" ",
		lipgloss.NewStyle().Width(columns.history).Render(s.muted.Render(historyLabel)),
	)
}

func renderComponent(component pulse.Component, history *pulse.ComponentHistory, width int, s styles) string {
	columns := componentColumnsFor(width)
	status := lipgloss.NewStyle().Width(columns.status).Render(s.status(component.State))
	if history == nil {
		return status + " " + between(component.Name, s.muted.Render("--"), width-columns.status-1)
	}
	uptime := "--"
	if history.Uptime90Days != nil {
		uptime = fmt.Sprintf("%.2f%%", *history.Uptime90Days)
	}
	visibleDays := history.Days90
	if len(visibleDays) > 30 {
		visibleDays = visibleDays[len(visibleDays)-30:]
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		status,
		" ",
		lipgloss.NewStyle().Width(columns.component).Render(truncate(component.Name, columns.component)),
		" ",
		lipgloss.NewStyle().Width(columns.uptime).Align(lipgloss.Right).Render(uptime),
		" ",
		renderHistoryStrip(visibleDays, columns.history, s),
	)
}

type componentColumnWidths struct {
	status, component, uptime, history int
}

func componentColumnsFor(width int) componentColumnWidths {
	const statusWidth = 8
	const uptimeWidth = 10
	const preferredComponentWidth = 26
	const gaps = 3
	historyWidth := min(30, max(8, width-statusWidth-uptimeWidth-preferredComponentWidth-gaps))
	return componentColumnWidths{
		status: statusWidth, component: width - statusWidth - uptimeWidth - historyWidth - gaps,
		uptime: uptimeWidth, history: historyWidth,
	}
}

func findComponentHistory(histories []pulse.ComponentHistory, name string) *pulse.ComponentHistory {
	for index := range histories {
		if histories[index].Name == name {
			return &histories[index]
		}
	}
	return nil
}
