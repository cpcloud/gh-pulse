// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package feed

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"time"

	"github.com/cpcloud/gh-pulse/internal/httpx"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

type Client struct {
	get *httpx.Getter
	url string
}

func New(get *httpx.Getter, url string) *Client { return &Client{get: get, url: url} }

type atomFeed struct {
	Updated time.Time   `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published *time.Time `xml:"published"`
	Updated   time.Time  `xml:"updated"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

func (c *Client) Fetch(ctx context.Context) (pulse.Feed, error) {
	data, err := c.get.Get(ctx, c.url)
	if err != nil {
		return pulse.Feed{}, fmt.Errorf("recent feed: %w", err)
	}
	var raw atomFeed
	if err := xml.Unmarshal(data, &raw); err != nil {
		return pulse.Feed{}, fmt.Errorf("recent feed: decode Atom: %w", err)
	}
	entries := make([]pulse.FeedEntry, 0, len(raw.Entries))
	for index, entry := range raw.Entries {
		if entry.Updated.IsZero() {
			return pulse.Feed{}, fmt.Errorf("recent feed: entry %d missing updated timestamp", index+1)
		}
		var url *string
		for _, link := range entry.Links {
			if link.Href != "" && (link.Rel == "alternate" || link.Rel == "") {
				href := link.Href
				url = &href
				break
			}
		}
		published := utcPtr(entry.Published)
		entries = append(entries, pulse.FeedEntry{
			ID: entry.ID, Title: entry.Title, URL: url,
			PublishedAt: published, UpdatedAt: entry.Updated.UTC(),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].UpdatedAt.After(entries[j].UpdatedAt) })
	return pulse.Feed{UpdatedAt: raw.Updated.UTC(), Entries: entries}, nil
}

func utcPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
