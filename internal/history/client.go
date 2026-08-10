// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cpcloud/gh-pulse/internal/httpx"
)

type Impact string

const (
	ImpactNone        Impact = "none"
	ImpactMaintenance Impact = "maintenance"
	ImpactMinor       Impact = "minor"
	ImpactMajor       Impact = "major"
	ImpactCritical    Impact = "critical"
)

type Interval struct {
	ID         string
	Start      time.Time
	End        time.Time
	Title      string
	Impact     Impact
	Components []string
}

type Client struct {
	get *httpx.Getter
	url string
}

func New(get *httpx.Getter, url string) *Client { return &Client{get: get, url: url} }

func (c *Client) Fetch(ctx context.Context) ([]Interval, error) {
	data, err := c.get.Get(ctx, c.url)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	intervals, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("history: decode JSONL: %w", err)
	}
	return intervals, nil
}

func decode(data []byte) ([]Interval, error) {
	type record struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		DowntimeStart *string  `json:"downtime_start"`
		DowntimeEnd   *string  `json:"downtime_end"`
		Impact        string   `json:"impact"`
		Components    []string `json:"components"`
	}

	intervals := make([]Interval, 0)
	sawRecord := false
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for recordNumber := 1; scanner.Scan(); recordNumber++ {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		sawRecord = true
		var value record
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, fmt.Errorf("record %d: %w", recordNumber, err)
		}
		if value.DowntimeStart == nil && value.DowntimeEnd == nil {
			continue
		}
		if value.DowntimeStart == nil || value.DowntimeEnd == nil {
			return nil, fmt.Errorf("record %d has only one downtime bound", recordNumber)
		}
		start, err := time.Parse(time.RFC3339, *value.DowntimeStart)
		if err != nil {
			return nil, fmt.Errorf("record %d start timestamp: %w", recordNumber, err)
		}
		end, err := time.Parse(time.RFC3339, *value.DowntimeEnd)
		if err != nil {
			return nil, fmt.Errorf("record %d end timestamp: %w", recordNumber, err)
		}
		if end.Before(start) {
			return nil, fmt.Errorf("record %d ends before it starts", recordNumber)
		}
		impact := Impact(value.Impact)
		if impact == "" {
			impact = ImpactNone
		}
		if !validImpact(impact) {
			return nil, fmt.Errorf("record %d has unsupported impact %q", recordNumber, impact)
		}
		intervals = append(intervals, Interval{
			ID: value.ID, Start: start.UTC(), End: end.UTC(), Title: value.Title,
			Impact: impact, Components: value.Components,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan records: %w", err)
	}
	if !sawRecord {
		return nil, fmt.Errorf("dataset is empty")
	}
	if len(intervals) == 0 {
		return nil, fmt.Errorf("dataset contains records but no reconstructed intervals")
	}
	return intervals, nil
}

func validImpact(impact Impact) bool {
	switch impact {
	case ImpactNone, ImpactMaintenance, ImpactMinor, ImpactMajor, ImpactCritical:
		return true
	default:
		return false
	}
}
