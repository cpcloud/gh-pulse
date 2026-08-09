package tui

import (
	"time"

	"charm.land/lipgloss/v2"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

const (
	colorText   = "#e7ecf4"
	colorMuted  = "#8791a3"
	colorBorder = "#343d4b"
)

type styles struct {
	mono         bool
	location     *time.Location
	title, muted lipgloss.Style
}

func newStyles(mono bool) styles {
	base := styles{
		mono:     mono,
		location: time.Local,
		title:    lipgloss.NewStyle().Bold(true),
		muted:    lipgloss.NewStyle().Faint(true),
	}
	if !mono {
		base.title = base.title.Foreground(lipgloss.Color(colorText))
		base.muted = base.muted.Foreground(lipgloss.Color(colorMuted))
	}
	return base
}

func (s styles) timestamp(value time.Time, layout string) string {
	return value.In(s.location).Format(layout)
}

func (s styles) panel(width int) lipgloss.Style {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(max(1, width))
	if !s.mono {
		style = style.BorderForeground(lipgloss.Color(colorBorder))
	}
	return style
}

func (s styles) keycap(key string) string {
	return lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1).Render(key)
}

func (s styles) heroMetric(state pulse.State) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	if !s.mono {
		style = style.Foreground(lipgloss.Color("#09110c")).Background(lipgloss.Color(stateColor(state))).Reverse(false)
	}
	return style
}

func (s styles) state(state pulse.State) lipgloss.Style {
	style := lipgloss.NewStyle().Bold(state != pulse.Operational)
	if !s.mono {
		style = style.Foreground(lipgloss.Color(stateColor(state)))
	}
	return style
}

func (s styles) status(state pulse.State) string {
	return s.state(state).Bold(true).Render(stateLabel(state))
}

func stateColor(state pulse.State) string {
	switch state {
	case pulse.Operational:
		return "#47c96b"
	case pulse.Maintenance:
		return "#63a9e8"
	case pulse.Minor:
		return "#d8a52b"
	case pulse.Major:
		return "#f0883e"
	case pulse.Critical:
		return "#f04444"
	default:
		return "#8b95a7"
	}
}

func stateLabel(state pulse.State) string {
	labels := map[pulse.State]string{
		pulse.Operational: "OK",
		pulse.Maintenance: "MAINT",
		pulse.Minor:       "DEGRADED",
		pulse.Major:       "PARTIAL",
		pulse.Critical:    "OUTAGE",
		pulse.Unknown:     "UNKNOWN",
	}
	return labels[state]
}
