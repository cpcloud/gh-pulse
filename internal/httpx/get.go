// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type Getter struct {
	client    *http.Client
	userAgent string
	limit     int64
}

func New(client *http.Client, userAgent string, limit int64) *Getter {
	return &Getter{client: client, userAgent: userAgent, limit: limit}
}

func (g *Getter) Get(ctx context.Context, url string) ([]byte, error) {
	if g.limit <= 0 {
		return nil, fmt.Errorf("invalid response limit %d", g.limit)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", g.userAgent)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, g.limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(data)) > g.limit {
		return nil, fmt.Errorf("response exceeds %d-byte limit", g.limit)
	}
	return data, nil
}
