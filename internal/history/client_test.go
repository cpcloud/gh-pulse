// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cpcloud/gh-pulse/internal/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDecodesGeneratedIncidentsAndAffectedComponents(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/incidents.jsonl")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 1<<20), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "old", got[0].ID)
	assert.Equal(t, []string{"Git Operations", "API Requests"}, got[0].Components)
	assert.Equal(t, ImpactNone, got[2].Impact)
}

func TestClientRejectsReversedInterval(t *testing.T) {
	t.Parallel()
	body := `{"id":"x","title":"bad","downtime_start":"2024-01-02T00:00:00Z","downtime_end":"2024-01-01T00:00:00Z","impact":"minor"}` + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "history")
}

func TestClientRejectsUnknownImpactMalformedJSONAndOneSidedBounds(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"impact":    `{"id":"x","downtime_start":"2024-01-01T00:00:00Z","downtime_end":"2024-01-02T00:00:00Z","impact":"mystery"}` + "\n",
		"json":      "{\n",
		"one-sided": `{"id":"x","downtime_start":"2024-01-01T00:00:00Z","impact":"minor"}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
			require.Error(t, err)
		})
	}
}

func TestClientSkipsIncidentsWithoutReconstructedDowntime(t *testing.T) {
	t.Parallel()
	body := `{"id":"no-window","title":"informational","impact":"none","components":["Issues"]}` + "\n" +
		`{"id":"window","title":"incident","downtime_start":"2024-01-01T00:00:00Z","downtime_end":"2024-01-01T01:00:00Z","impact":"minor","components":["Issues"]}` + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "window", got[0].ID)
}

func TestClientRejectsEmptyDataset(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataset is empty")
}

func TestClientRejectsDatasetWithOnlyUnboundedRecords(t *testing.T) {
	t.Parallel()
	body := `{"id":"no-window","title":"informational","impact":"none","components":["Issues"]}` + "\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "records but no reconstructed intervals")
}
