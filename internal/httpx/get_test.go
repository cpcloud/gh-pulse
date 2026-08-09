package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSendsIdentityWithoutCredentials(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "gh-pulse/test", r.Header.Get("User-Agent"))
		for _, name := range []string{"Authorization", "Cookie", "X-GitHub-Token"} {
			assert.Empty(t, r.Header.Get(name))
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	getter := New(server.Client(), "gh-pulse/test", 16)
	got, err := getter.Get(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(got))
}

func TestGetRejectsErrorBodyWithoutLeakingIt(t *testing.T) {
	t.Parallel()

	const secretBody = "upstream-private-diagnostic"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secretBody, http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := New(server.Client(), "gh-pulse/test", 1024).Get(context.Background(), server.URL)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secretBody)
}

func TestGetRejectsResponsePastLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("too large"))
	}))
	defer server.Close()

	_, err := New(server.Client(), "gh-pulse/test", 3).Get(context.Background(), server.URL)
	require.Error(t, err)
}

func TestGetRejectsInvalidLimitBeforeRequest(t *testing.T) {
	t.Parallel()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := New(server.Client(), "gh-pulse/test", 0).Get(context.Background(), server.URL)
	require.Error(t, err)
	assert.False(t, called)
	assert.Contains(t, err.Error(), "invalid response limit")
}

func TestGetStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := New(server.Client(), "gh-pulse/test", 16).Get(ctx, server.URL)
		done <- err
	}()
	<-started
	cancel()
	require.Error(t, <-done)
}
