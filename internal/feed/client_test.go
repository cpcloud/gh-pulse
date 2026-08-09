package feed

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

func TestClientDecodesStructuredAtomFieldsNewestFirst(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("testdata/history.atom")
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()

	got, err := New(httpx.New(server.Client(), "gh-pulse/test", 1<<20), server.URL).Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "New incident", got.Entries[0].Title)
	require.NotNil(t, got.Entries[0].URL)
	assert.Equal(t, "https://www.githubstatus.com/incidents/new", *got.Entries[0].URL)
	assert.Nil(t, got.Entries[1].URL)
}

func TestClientLabelsMalformedXMLWithoutLeakingIt(t *testing.T) {
	t.Parallel()
	const body = "<feed>sensitive-invalid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recent feed")
	assert.NotContains(t, err.Error(), body)
}

func TestClientRejectsEntryWithoutUpdatedTimestamp(t *testing.T) {
	t.Parallel()
	const body = `<feed xmlns="http://www.w3.org/2005/Atom"><updated>2026-08-09T14:00:00Z</updated><entry><id>x</id><title>Incident</title></entry></feed>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()

	_, err := New(httpx.New(server.Client(), "gh-pulse/test", 4096), server.URL).Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry 1")
	assert.Contains(t, err.Error(), "updated timestamp")
}
