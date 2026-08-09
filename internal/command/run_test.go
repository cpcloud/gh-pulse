package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fetchStub struct{ value pulse.Snapshot }

func (f fetchStub) Fetch(context.Context) pulse.Snapshot { return f.value }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed output") }

func TestRunJSONEmitsStableDocumentWithoutTerminalProse(t *testing.T) {
	t.Parallel()
	value := pulse.Snapshot{SchemaVersion: 1, Overall: pulse.Overall{State: pulse.Operational}, Components: []pulse.Component{}, ActiveIncidents: []pulse.Incident{}, ActiveMaintenances: []pulse.MaintenanceWindow{}, RecentFeed: []pulse.FeedEntry{}, Errors: []pulse.SourceError{}}
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{"--json"}, &stdout, &stderr, "test", fetchStub{value}, func() error { return nil })
	assert.Zero(t, exit)
	assert.Empty(t, stderr.String())
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
	assert.InDelta(t, 1, decoded["schema_version"], 0)
	assert.NotContains(t, stdout.String(), "\x1b[")
}

func TestRunJSONExitsNonzeroWhenCurrentSourceFailed(t *testing.T) {
	t.Parallel()
	value := pulse.Snapshot{SchemaVersion: 1, Overall: pulse.Overall{State: pulse.Unknown}, Errors: []pulse.SourceError{{Source: "current", Message: "unavailable"}}}
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{"--json"}, &stdout, &stderr, "test", fetchStub{value}, func() error { return nil })
	assert.Equal(t, 1, exit)
	assert.NotEmpty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunJSONExitsNonzeroWhenWritingOutputFails(t *testing.T) {
	t.Parallel()
	value := pulse.Snapshot{SchemaVersion: 1, Errors: []pulse.SourceError{}}
	var stderr bytes.Buffer
	exit := Run(context.Background(), []string{"--json"}, failingWriter{}, &stderr, "test", fetchStub{value}, func() error { return nil })
	assert.Equal(t, 1, exit)
	assert.Contains(t, stderr.String(), "write JSON")
	assert.Contains(t, stderr.String(), "closed output")
}

func TestRunRejectsUnknownOptionWithoutStartingTUI(t *testing.T) {
	t.Parallel()
	started := false
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{"--wat"}, &stdout, &stderr, "test", fetchStub{}, func() error { started = true; return nil })
	assert.Equal(t, 2, exit)
	assert.False(t, started)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "unknown option")
}

func TestRunHelpOnlyListsSupportedKeys(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer

	exit := Run(context.Background(), []string{"--help"}, &stdout, &stderr, "test", fetchStub{}, func() error { return nil })

	assert.Zero(t, exit)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "q to quit")
	assert.Contains(t, stdout.String(), "r to refresh")
	assert.NotContains(t, stdout.String(), "Tab")
}
