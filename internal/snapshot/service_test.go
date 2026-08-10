// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cpcloud/gh-pulse/internal/history"
	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type currentStub struct {
	value pulse.Current
	err   error
}

func (s currentStub) Fetch(context.Context) (pulse.Current, error) { return s.value, s.err }

type feedStub struct {
	value pulse.Feed
	err   error
}

func (s feedStub) Fetch(context.Context) (pulse.Feed, error) { return s.value, s.err }

type historyStub struct {
	value []history.Interval
	err   error
}

func (s historyStub) Fetch(context.Context) ([]history.Interval, error) { return s.value, s.err }

func TestFetchKeepsCurrentStatusWhenOptionalSourcesFail(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	updated := now.Add(-time.Minute)
	service := New(
		currentStub{value: pulse.Current{Overall: pulse.Overall{State: pulse.Operational, Description: "All Systems Operational", UpdatedAt: &updated}}},
		feedStub{err: errors.New("recent feed unavailable")}, historyStub{err: errors.New("history unavailable")},
		Config{CurrentURL: "current", FeedURL: "feed", HistoryURL: "history"}, func() time.Time { return now },
	)
	got := service.Fetch(context.Background())
	assert.Equal(t, pulse.Operational, got.Overall.State)
	assert.True(t, got.Sources.Current.Available)
	assert.False(t, got.Sources.Feed.Available)
	assert.Equal(t, []string{"feed", "history"}, []string{got.Errors[0].Source, got.Errors[1].Source})
	assert.Equal(t, &now, got.Errors[0].AttemptedAt)
	assert.Equal(t, &now, got.Errors[1].AttemptedAt)
	assert.Empty(t, got.RecentFeed)
}

func TestFetchLiveLeavesMissingFeedUpdateTimeNull(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	service := New(
		currentStub{value: pulse.Current{Overall: pulse.Overall{State: pulse.Operational}}},
		feedStub{value: pulse.Feed{}}, historyStub{},
		Config{CurrentURL: "current", FeedURL: "feed", HistoryURL: "history"}, func() time.Time { return now },
	)

	got := service.FetchLive(context.Background())
	assert.True(t, got.Sources.Feed.Available)
	assert.Nil(t, got.Sources.Feed.UpdatedAt)
}

func TestFetchIncludesCalculatedHistoryFromSuccessfulSource(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -2)
	service := New(
		currentStub{value: pulse.Current{Overall: pulse.Overall{State: pulse.Operational}}},
		feedStub{value: pulse.Feed{}},
		historyStub{value: []history.Interval{{Start: start, End: start.Add(time.Hour), Impact: history.ImpactMinor}}},
		Config{CurrentURL: "current", FeedURL: "feed", HistoryURL: "history"}, func() time.Time { return now },
	)

	got := service.Fetch(context.Background())
	assert.True(t, got.Sources.History.Available)
	assert.Equal(t, &now, got.Sources.History.FetchedAt)
	require.NotNil(t, got.History.Uptime90Days)
	assert.Less(t, *got.History.Uptime90Days, 100.0)
	assert.Empty(t, got.Errors)
}

func TestWriteJSONIsOneAgentSafeDocument(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	uptime := 99.99
	downtimeDays := 5.94
	attempted := time.Date(2026, 8, 9, 14, 0, 0, 0, time.UTC)
	value := pulse.Snapshot{
		SchemaVersion: 1, Components: []pulse.Component{}, ActiveIncidents: []pulse.Incident{},
		ActiveMaintenances: []pulse.MaintenanceWindow{}, RecentFeed: []pulse.FeedEntry{},
		Errors: []pulse.SourceError{{Source: "history", Message: "unavailable", AttemptedAt: &attempted}},
		History: pulse.History{
			Downtime90Days: &downtimeDays,
			Components:     []pulse.ComponentHistory{{Name: "Git Operations", Uptime90Days: &uptime, Days90: []pulse.Day{}}},
		},
	}
	require.NoError(t, WriteJSON(&output, value))
	assert.NotContains(t, output.String(), "\x1b[")
	assert.Equal(t, byte('\n'), output.Bytes()[output.Len()-1])
	assert.Contains(t, output.String(), `"schema_version":1`)
	assert.NotContains(t, output.String(), "attempted_at")
	var decoded struct {
		History struct {
			Downtime90Days *float64                 `json:"downtime_90_days"`
			Components     []pulse.ComponentHistory `json:"components"`
		} `json:"history"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &decoded))
	require.Len(t, decoded.History.Components, 1)
	require.NotNil(t, decoded.History.Downtime90Days)
	assert.InDelta(t, 5.94, *decoded.History.Downtime90Days, 0.000001)
	assert.Equal(t, "Git Operations", decoded.History.Components[0].Name)
	assert.InDelta(t, 99.99, *decoded.History.Components[0].Uptime90Days, 0.000001)
}
