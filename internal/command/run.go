// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cpcloud/gh-pulse/internal/feed"
	"github.com/cpcloud/gh-pulse/internal/history"
	"github.com/cpcloud/gh-pulse/internal/httpx"
	"github.com/cpcloud/gh-pulse/internal/pulse"
	"github.com/cpcloud/gh-pulse/internal/snapshot"
	"github.com/cpcloud/gh-pulse/internal/statuspage"
	"github.com/cpcloud/gh-pulse/internal/tui"
)

const (
	currentURL = "https://www.githubstatus.com/api/v2/summary.json"
	feedURL    = "https://www.githubstatus.com/history.atom"
	historyURL = "https://raw.githubusercontent.com/mrshu/github-statuses/master/parsed/incidents.jsonl"
	// Oversized public sources fail unavailable instead of being truncated into misleading status.
	responseLimit = 8 << 20
)

type SnapshotFetcher interface {
	Fetch(context.Context) pulse.Snapshot
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string, fetcher SnapshotFetcher, startTUI func() error) int {
	if len(args) == 0 {
		if err := startTUI(); err != nil {
			_, _ = fmt.Fprintf(stderr, "gh-pulse: %v\n", err)
			return 1
		}
		return 0
	}
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, "gh-pulse: expected at most one option")
		return 2
	}
	switch args[0] {
	case "--json":
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		value := fetcher.Fetch(requestCtx)
		if err := snapshot.WriteJSON(stdout, value); err != nil {
			_, _ = fmt.Fprintf(stderr, "gh-pulse: write JSON: %v\n", err)
			return 1
		}
		for _, sourceErr := range value.Errors {
			if sourceErr.Source == "current" {
				return 1
			}
		}
		return 0
	case "--help", "-h":
		_, _ = fmt.Fprintln(stdout, "Usage: gh pulse [--json]\n\nShow GitHub service health. Press q to quit or r to refresh.")
		return 0
	case "--version":
		_, _ = fmt.Fprintf(stdout, "gh-pulse %s\n", version)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "gh-pulse: unknown option %q\n", args[0])
		return 2
	}
}

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	client := &http.Client{Transport: http.DefaultTransport}
	getter := httpx.New(client, "gh-pulse/"+version, responseLimit)
	service := snapshot.New(
		statuspage.New(getter, currentURL), feed.New(getter, feedURL), history.New(getter, historyURL),
		snapshot.Config{CurrentURL: currentURL, FeedURL: feedURL, HistoryURL: historyURL}, time.Now,
	)
	start := func() error {
		_, err := tea.NewProgram(tui.New(service, os.Getenv("NO_COLOR") != "")).Run()
		return err
	}
	return Run(ctx, args, stdout, stderr, version, service, start)
}
