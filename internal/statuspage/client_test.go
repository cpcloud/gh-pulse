// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package statuspage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cpcloud/gh-pulse/internal/httpx"
	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientNormalizesVisibleCurrentStatus(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/summary.json")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 1<<20), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, pulse.Major, got.Overall.State)
	assert.Equal(t, "Major service disruption", got.Overall.Description)
	require.Len(t, got.Components, 3)
	assert.Equal(t, []string{"group", "child", "missing"}, []string{got.Components[0].ID, got.Components[1].ID, got.Components[2].ID})
	assert.Nil(t, got.Components[2].Description)
	require.NotNil(t, got.ActiveIncidents[0].LatestUpdate)
	assert.Equal(t, "Recovery is being monitored.", got.ActiveIncidents[0].LatestUpdate.Body)
	assert.Nil(t, got.ActiveIncidents[1].LatestUpdate)
	require.Len(t, got.ActiveMaintenances, 2)
	assert.Equal(t, []string{"active", "later"}, []string{got.ActiveMaintenances[0].ID, got.ActiveMaintenances[1].ID})
	assert.Nil(t, got.ActiveMaintenances[1].ScheduledUntil)
}

func TestClientExcludesResolvedIncidentsFromActiveState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":{"updated_at":"2026-08-09T00:00:00Z"},"components":[],"incidents":[{"id":"done","name":"Resolved outage","status":"resolved","impact":"major","updated_at":"2026-08-09T00:00:00Z","incident_updates":[]}],"scheduled_maintenances":[],"status":{"indicator":"none","description":"All Systems Operational"}}`))
	}))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, pulse.Operational, got.Overall.State)
	assert.Empty(t, got.ActiveIncidents)
}

func TestClientMapsMissingPageTimestampToNil(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":{},"components":[],"incidents":[],"scheduled_maintenances":[],"status":{"indicator":"none","description":"All Systems Operational"}}`))
	}))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got.Overall.UpdatedAt)
}

func TestClientRejectsUnknownHiddenComponentStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":{"updated_at":"2026-08-09T00:00:00Z"},"components":[{"id":"hidden","name":"Hidden","status":"mystery","position":1,"showcase":false}],"incidents":[],"scheduled_maintenances":[],"status":{"indicator":"none","description":"Fine"}}`))
	}))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported component status")
}

func TestClientRejectsUnknownComponentStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":{"updated_at":"2026-08-09T00:00:00Z"},"components":[{"id":"x","name":"X","status":"mystery","position":1}],"incidents":[],"scheduled_maintenances":[],"status":{"indicator":"none","description":"Fine"}}`))
	}))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "current status")
}

func TestClientRejectsUnknownMaintenanceStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"page":{"updated_at":"2026-08-09T00:00:00Z"},"components":[],"incidents":[],"scheduled_maintenances":[{"id":"x","name":"Maintenance","status":"mystery","updated_at":"2026-08-09T00:00:00Z"}],"status":{"indicator":"none","description":"Fine"}}`))
	}))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported maintenance status")
	assert.Contains(t, err.Error(), "current status")
}

func TestClientLabelsMalformedJSONWithoutEchoingIt(t *testing.T) {
	t.Parallel()

	const body = "not-json-sensitive-body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "current status")
	assert.NotContains(t, err.Error(), body)
}
