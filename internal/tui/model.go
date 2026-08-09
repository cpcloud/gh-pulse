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
		width: 80, height: 24, mono: monochrome, now: time.Now, errors: map[string]pulse.SourceError{},
	}
}

func (m *Model) Init() tea.Cmd {
	m.nextRefresh = m.now().Add(refreshEvery)
	refresh := m.startFullRefresh()
	m.syncView()
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
		m.syncView()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelAll()
			return m, tea.Quit
		case "r":
			m.nextRefresh = m.now().Add(refreshEvery)
			refresh := m.startFullRefresh()
			return m, refresh
		case "j", "down":
			m.view.ScrollDown(1)
		case "k", "up":
			m.view.ScrollUp(1)
		case "pgdown":
			m.view.PageDown()
		case "pgup":
			m.view.PageUp()
		}
	case tickMsg:
		now := time.Time(msg)
		if m.nextRefresh.IsZero() {
			m.nextRefresh = now.Add(refreshEvery)
		}
		if now.Before(m.nextRefresh) {
			m.syncView()
			return m, tick()
		}
		m.nextRefresh = now.Add(refreshEvery)
		var refresh tea.Cmd
		if m.historyNeedsRefresh(now) {
			refresh = m.startFullRefresh()
		} else {
			refresh = m.startLive()
		}
		m.syncView()
		return m, tea.Batch(refresh, tick())
	case liveMsg:
		if msg.generation == m.liveGen {
			m.mergeLive(msg.value)
			m.nextRefresh = m.now().Add(refreshEvery)
			m.syncView()
		}
	case fullMsg:
		if msg.generation == m.fullGen {
			m.fullLoading = false
			m.mergeLive(msg.value)
			m.mergeHistory(msg.value)
			m.ready = true
			m.nextRefresh = m.now().Add(refreshEvery)
			m.syncView()
		}
	default:
		m.view, command = m.view.Update(message)
	}
	return m, command
}

func (m *Model) View() tea.View {
	view := tea.NewView(m.view.View())
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
		countdown: max(time.Duration(0), m.nextRefresh.Sub(m.now())),
	}
	content := render(m.data, options)
	if lipgloss.Height(content) > m.height {
		options.scrollable = true
		content = render(m.data, options)
	}
	content = lipgloss.PlaceVertical(m.height, lipgloss.Center, content)
	m.view.SetContent(content)
	m.view.SetYOffset(y)
}
