package snapshot

import (
	"context"
	"time"

	"github.com/cpcloud/gh-pulse/internal/history"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

type CurrentClient interface {
	Fetch(context.Context) (pulse.Current, error)
}
type FeedClient interface {
	Fetch(context.Context) (pulse.Feed, error)
}
type HistoryClient interface {
	Fetch(context.Context) ([]history.Interval, error)
}

type Config struct {
	CurrentURL string
	FeedURL    string
	HistoryURL string
}

type Service struct {
	current CurrentClient
	feed    FeedClient
	history HistoryClient
	config  Config
	now     func() time.Time
}

func New(current CurrentClient, feed FeedClient, archive HistoryClient, config Config, now func() time.Time) *Service {
	return &Service{current: current, feed: feed, history: archive, config: config, now: now}
}

type currentResult struct {
	value pulse.Current
	err   error
}
type feedResult struct {
	value pulse.Feed
	err   error
}

func (s *Service) Fetch(ctx context.Context) pulse.Snapshot {
	generated := s.now().UTC()
	liveCh := make(chan pulse.Snapshot, 1)
	archiveCh := make(chan pulse.Snapshot, 1)
	go func() { liveCh <- s.fetchLive(ctx, generated) }()
	go func() { archiveCh <- s.fetchArchive(ctx, generated) }()
	live, archive := <-liveCh, <-archiveCh
	live.History = archive.History
	live.Sources.History = archive.Sources.History
	live.Errors = append(live.Errors, archive.Errors...)
	return live
}

func (s *Service) FetchLive(ctx context.Context) pulse.Snapshot {
	return s.fetchLive(ctx, s.now().UTC())
}

func (s *Service) fetchLive(ctx context.Context, generated time.Time) pulse.Snapshot {
	currentCh := make(chan currentResult, 1)
	feedCh := make(chan feedResult, 1)
	go func() { value, err := s.current.Fetch(ctx); currentCh <- currentResult{value, err} }()
	go func() { value, err := s.feed.Fetch(ctx); feedCh <- feedResult{value, err} }()

	current, recent := <-currentCh, <-feedCh
	result := pulse.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated,
		Overall:    pulse.Overall{State: pulse.Unknown, Description: "GitHub status unavailable"},
		Components: []pulse.Component{}, ActiveIncidents: []pulse.Incident{},
		ActiveMaintenances: []pulse.MaintenanceWindow{}, RecentFeed: []pulse.FeedEntry{},
		History: emptyHistory(generated),
		Sources: pulse.Sources{
			Current: pulse.Source{URL: s.config.CurrentURL},
			Feed:    pulse.Source{URL: s.config.FeedURL},
			History: pulse.Source{URL: s.config.HistoryURL},
		},
		Errors: []pulse.SourceError{},
	}

	if current.err != nil {
		result.Errors = append(result.Errors, pulse.SourceError{Source: "current", Message: current.err.Error(), AttemptedAt: timePtr(generated)})
	} else {
		result.Overall = current.value.Overall
		result.Components = nonNil(current.value.Components)
		result.ActiveIncidents = nonNil(current.value.ActiveIncidents)
		result.ActiveMaintenances = nonNil(current.value.ActiveMaintenances)
		result.Sources.Current.Available = true
		result.Sources.Current.FetchedAt = timePtr(generated)
		result.Sources.Current.UpdatedAt = current.value.Overall.UpdatedAt
	}
	if recent.err != nil {
		result.Errors = append(result.Errors, pulse.SourceError{Source: "feed", Message: recent.err.Error(), AttemptedAt: timePtr(generated)})
	} else {
		result.RecentFeed = nonNil(recent.value.Entries)
		result.Sources.Feed.Available = true
		result.Sources.Feed.FetchedAt = timePtr(generated)
		result.Sources.Feed.UpdatedAt = nonZeroTimePtr(recent.value.UpdatedAt)
	}
	return result
}

func (s *Service) fetchArchive(ctx context.Context, generated time.Time) pulse.Snapshot {
	archive, err := s.history.Fetch(ctx)
	result := pulse.Snapshot{
		SchemaVersion: 1, GeneratedAt: generated, History: emptyHistory(generated),
		Sources: pulse.Sources{History: pulse.Source{URL: s.config.HistoryURL}}, Errors: []pulse.SourceError{},
	}
	if err != nil {
		result.Errors = append(result.Errors, pulse.SourceError{Source: "history", Message: err.Error(), AttemptedAt: timePtr(generated)})
	} else if calculated, calculateErr := history.Calculate(archive, generated); calculateErr != nil {
		result.Errors = append(result.Errors, pulse.SourceError{Source: "history", Message: calculateErr.Error(), AttemptedAt: timePtr(generated)})
	} else {
		result.History = calculated
		result.Sources.History.Available = true
		result.Sources.History.FetchedAt = timePtr(generated)
	}
	return result
}

func emptyHistory(now time.Time) pulse.History {
	utc := now.UTC()
	asOf := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return pulse.History{
		Source: "mrshu/github-statuses", CoverageStart: history.CoverageStart, AsOf: asOf,
		Days90: []pulse.Day{}, Rolling90Days: pulse.RollingHistory{Series: []pulse.RollingPoint{}}, Components: []pulse.ComponentHistory{},
	}
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

func timePtr(value time.Time) *time.Time { value = value.UTC(); return &value }

func nonZeroTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return timePtr(value)
}
