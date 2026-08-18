// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

const refreshEvery = time.Minute
const countdownEvery = time.Second
const requestTimeout = 8 * time.Second

type Fetcher interface {
	Fetch(context.Context) pulse.Snapshot
	FetchLive(context.Context) pulse.Snapshot
}

type Model struct {
	fetcher     Fetcher
	view        viewport.Model
	detailView  viewport.Model
	data        pulse.Snapshot
	width       int
	height      int
	mono        bool
	liveGen     uint64
	fullGen     uint64
	liveCancel  context.CancelFunc
	fullCancel  context.CancelFunc
	fullLoading bool
	ready       bool
	nextRefresh time.Time
	now         func() time.Time
	errors      map[string]pulse.SourceError
	detailOpen  bool
	detailID    string
	detailIndex int
	feedOffset  int
}

type liveMsg struct {
	generation uint64
	value      pulse.Snapshot
}
type fullMsg struct {
	generation uint64
	value      pulse.Snapshot
}
type tickMsg time.Time

func New(fetcher Fetcher, monochrome bool) *Model {
	return &Model{
		fetcher: fetcher, view: viewport.New(viewport.WithWidth(80), viewport.WithHeight(24)),
		detailView: viewport.New(viewport.WithWidth(68), viewport.WithHeight(12)),
		width:      80, height: 24, mono: monochrome, now: time.Now, errors: map[string]pulse.SourceError{},
	}
}

func (m *Model) Init() tea.Cmd {
	now := m.now()
	m.nextRefresh = now.Add(refreshEvery)
	refresh := m.startFullRefresh()
	m.syncViewAt(now)
	return tea.Batch(refresh, tick())
}

func tick() tea.Cmd {
	return tea.Tick(countdownEvery, func(value time.Time) tea.Msg { return tickMsg(value) })
}

func (m *Model) startLive() tea.Cmd {
	if m.liveCancel != nil {
		m.liveCancel()
	}
	m.liveGen++
	generation := m.liveGen
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	m.liveCancel = cancel
	return func() tea.Msg { defer cancel(); return liveMsg{generation, m.fetcher.FetchLive(ctx)} }
}

func (m *Model) startFullRefresh() tea.Cmd {
	if m.liveCancel != nil {
		m.liveCancel()
	}
	if m.fullCancel != nil {
		m.fullCancel()
	}
	m.liveGen++
	m.fullGen++
	generation := m.fullGen
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	m.fullCancel = cancel
	m.fullLoading = true
	return func() tea.Msg {
		defer cancel()
		return fullMsg{generation: generation, value: m.fetcher.Fetch(ctx)}
	}
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(40, msg.Width), max(10, msg.Height)
		m.reconcileDetail()
		m.syncView()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancelAll()
			return m, tea.Quit
		case "esc":
			if m.detailOpen {
				m.closeDetail()
				return m, nil
			}
			m.cancelAll()
			return m, tea.Quit
		case "enter":
			if !m.detailOpen {
				m.openDetail(m.detailIndex)
			}
		case "left":
			if m.detailOpen {
				m.pageDetail(-1)
			}
		case "right":
			if m.detailOpen {
				m.pageDetail(1)
			}
		case "up":
			if m.detailOpen {
				m.detailView.ScrollUp(1)
			} else {
				m.selectDetail(m.detailIndex - 1)
			}
		case "down":
			if m.detailOpen {
				m.detailView.ScrollDown(1)
			} else {
				m.selectDetail(m.detailIndex + 1)
			}
		case "r":
			now := m.now()
			m.nextRefresh = now.Add(refreshEvery)
			refresh := m.startFullRefresh()
			return m, refresh
		case "j":
			if m.detailOpen {
				m.detailView.ScrollDown(1)
			} else {
				m.view.ScrollDown(1)
			}
		case "k":
			if m.detailOpen {
				m.detailView.ScrollUp(1)
			} else {
				m.view.ScrollUp(1)
			}
		case "pgdown":
			if m.detailOpen {
				m.detailView.PageDown()
			} else {
				m.view.PageDown()
			}
		case "pgup":
			if m.detailOpen {
				m.detailView.PageUp()
			} else {
				m.view.PageUp()
			}
		}
	case tickMsg:
		now := time.Time(msg)
		if m.nextRefresh.IsZero() {
			m.nextRefresh = now.Add(refreshEvery)
		}
		if now.Before(m.nextRefresh) {
			if !m.detailOpen {
				m.syncViewAt(now)
			}
			return m, tick()
		}
		m.nextRefresh = now.Add(refreshEvery)
		var refresh tea.Cmd
		if m.historyNeedsRefresh(now) {
			refresh = m.startFullRefresh()
		} else {
			refresh = m.startLive()
		}
		m.syncViewAt(now)
		return m, tea.Batch(refresh, tick())
	case liveMsg:
		if msg.generation == m.liveGen {
			now := m.now()
			m.mergeLive(msg.value)
			m.nextRefresh = now.Add(refreshEvery)
			m.syncViewAt(now)
		}
	case fullMsg:
		if msg.generation == m.fullGen {
			now := m.now()
			m.fullLoading = false
			m.mergeLive(msg.value)
			m.mergeHistory(msg.value)
			m.ready = true
			m.nextRefresh = now.Add(refreshEvery)
			m.syncViewAt(now)
		}
	default:
		m.view, command = m.view.Update(message)
	}
	return m, command
}

func (m *Model) View() tea.View {
	content := m.view.View()
	if entry, index, total, ok := m.detailEntry(); ok {
		s := newStyles(m.mono)
		overlay := renderEntryDetail(entry, index, total, detailRenderOptions{
			width: m.width, height: m.height, mono: m.mono, view: &m.detailView,
		})
		x := max(0, (m.width-lipgloss.Width(overlay))/2)
		y := max(0, (m.height-1-lipgloss.Height(overlay))/2)
		dashboardWidth := min(max(m.width-4, 36), 140)
		globalFooter := renderFooter(dashboardWidth, s, false, false)
		footerX := max(0, (m.width-lipgloss.Width(globalFooter))/2)
		footerY := y + lipgloss.Height(overlay)
		layers := lipgloss.NewCompositor(
			lipgloss.NewLayer(content),
			lipgloss.NewLayer(overlay).X(x).Y(y).Z(1),
			lipgloss.NewLayer(globalFooter).X(footerX).Y(footerY).Z(2),
		)
		content = lipgloss.NewCanvas(m.width, m.height).Compose(layers).Render()
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "GitHub Pulse"
	return view
}

func (m *Model) historyNeedsRefresh(now time.Time) bool {
	if m.fullLoading {
		return false
	}
	lastSuccess := m.data.Sources.History.FetchedAt
	if lastSuccess == nil {
		_, failed := m.errors["history"]
		return failed
	}
	lastUTC, nowUTC := lastSuccess.UTC(), now.UTC()
	return lastUTC.Year() != nowUTC.Year() || lastUTC.YearDay() != nowUTC.YearDay()
}

func (m *Model) cancelAll() {
	if m.liveCancel != nil {
		m.liveCancel()
	}
	if m.fullCancel != nil {
		m.fullCancel()
	}
}

func (m *Model) mergeLive(next pulse.Snapshot) {
	if next.Sources.Current.Available || !m.data.Sources.Current.Available {
		m.data.Overall, m.data.Components = next.Overall, next.Components
		m.data.ActiveIncidents, m.data.ActiveMaintenances = next.ActiveIncidents, next.ActiveMaintenances
		m.data.Sources.Current = next.Sources.Current
	}
	if next.Sources.Feed.Available || !m.data.Sources.Feed.Available {
		m.data.RecentFeed, m.data.Sources.Feed = next.RecentFeed, next.Sources.Feed
		m.reconcileDetail()
	}
	m.data.GeneratedAt = next.GeneratedAt
	m.replaceErrors(next.Errors, "current", "feed")
}

func (m *Model) mergeHistory(next pulse.Snapshot) {
	if next.Sources.History.Available || !m.data.Sources.History.Available {
		m.data.History, m.data.Sources.History = next.History, next.Sources.History
	}
	m.replaceErrors(next.Errors, "history")
}

func (m *Model) replaceErrors(values []pulse.SourceError, sources ...string) {
	for _, source := range sources {
		delete(m.errors, source)
	}
	for _, value := range values {
		m.errors[value.Source] = value
	}
	m.data.Errors = m.data.Errors[:0]
	for _, source := range []string{"current", "feed", "history"} {
		if sourceError, ok := m.errors[source]; ok {
			m.data.Errors = append(m.data.Errors, sourceError)
		}
	}
}

func (m *Model) syncView() {
	m.syncViewAt(m.now())
}

func (m *Model) syncViewAt(now time.Time) {
	m.reconcileDetail()
	y := m.view.YOffset()
	m.view.SetWidth(m.width)
	m.view.SetHeight(m.height)
	if !m.ready {
		m.view.SetContent("")
		m.view.SetYOffset(y)
		return
	}
	options := renderOptions{
		width: m.width, height: m.height, mono: m.mono,
		selectedFeed: m.detailIndex,
		feedOffset:   m.feedOffset,
		countdown:    max(time.Duration(0), m.nextRefresh.Sub(now)),
		now:          now,
		detailOpen:   m.detailOpen,
	}
	content := render(m.data, options)
	if lipgloss.Height(content) > m.height {
		options.scrollable = true
		content = render(m.data, options)
	}
	content = lipgloss.PlaceVertical(m.height, lipgloss.Center, content)
	m.view.SetContent(content)
	m.view.SetYOffset(y)
	m.syncDetailView()
}

func (m *Model) syncDetailView() {
	entry, _, _, ok := m.detailEntry()
	if !ok {
		return
	}
	layout := entryDetailLayout(m.width, m.height, newStyles(m.mono))
	configureDetailView(&m.detailView, entry, layout, m.mono, newStyles(m.mono))
}

func (m *Model) syncFeedOffset() {
	width := min(max(m.width-4, 36), 140)
	innerWidth := panelContentWidth(newStyles(m.mono), width)
	limit := feedEntryLimit(innerWidth, m.height)
	m.feedOffset, _ = feedWindow(len(m.data.RecentFeed), m.detailIndex, m.feedOffset, limit)
}

func (m *Model) detailEntry() (pulse.FeedEntry, int, int, bool) {
	if !m.detailOpen {
		return pulse.FeedEntry{}, 0, 0, false
	}
	entries := m.data.RecentFeed
	if m.detailIndex < 0 || m.detailIndex >= len(entries) {
		return pulse.FeedEntry{}, 0, 0, false
	}
	return entries[m.detailIndex], m.detailIndex, len(entries), true
}

func (m *Model) openDetail(index int) {
	entries := m.data.RecentFeed
	if index < 0 || index >= len(entries) {
		return
	}
	m.detailOpen = true
	m.detailIndex = index
	m.detailID = entries[index].ID
	m.detailView.GotoTop()
	m.syncView()
}

func (m *Model) selectDetail(index int) {
	entries := m.data.RecentFeed
	if index < 0 || index >= len(entries) {
		return
	}
	m.detailIndex = index
	m.detailID = entries[index].ID
	m.syncFeedOffset()
	m.syncView()
	m.view.GotoBottom()
}

func (m *Model) pageDetail(delta int) {
	index := m.detailIndex + delta
	if !m.detailOpen || index < 0 || index >= len(m.data.RecentFeed) {
		return
	}
	m.detailIndex = index
	m.detailID = m.data.RecentFeed[index].ID
	m.detailView.GotoTop()
	m.syncFeedOffset()
	m.syncView()
}

func (m *Model) closeDetail() {
	m.detailOpen = false
	m.syncView()
}

func (m *Model) reconcileDetail() {
	previousID := m.detailID
	entries := m.data.RecentFeed
	if len(entries) == 0 {
		m.detailOpen = false
		m.detailID = ""
		m.detailIndex = 0
		m.feedOffset = 0
		return
	}
	if m.detailID != "" {
		for index, entry := range entries {
			if entry.ID == m.detailID {
				m.detailIndex = index
				m.syncFeedOffset()
				return
			}
		}
	}
	m.detailIndex = 0
	m.detailID = entries[0].ID
	m.feedOffset = 0
	if m.detailOpen && previousID != m.detailID {
		m.detailView.GotoTop()
	}
}
