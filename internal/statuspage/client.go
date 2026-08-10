// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package statuspage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cpcloud/gh-pulse/internal/httpx"
	"github.com/cpcloud/gh-pulse/internal/pulse"
)

type Client struct {
	get *httpx.Getter
	url string
}

func New(get *httpx.Getter, url string) *Client {
	return &Client{get: get, url: url}
}

type response struct {
	Page struct {
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"page"`
	Components []struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Status      string  `json:"status"`
		Description *string `json:"description"`
		Position    int     `json:"position"`
		Showcase    *bool   `json:"showcase"`
		Group       bool    `json:"group"`
		GroupID     *string `json:"group_id"`
	} `json:"components"`
	Incidents []struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Status    string    `json:"status"`
		Impact    string    `json:"impact"`
		UpdatedAt time.Time `json:"updated_at"`
		Updates   []struct {
			Status    string    `json:"status"`
			Body      string    `json:"body"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"incident_updates"`
	} `json:"incidents"`
	Maintenances []struct {
		ID             string     `json:"id"`
		Name           string     `json:"name"`
		Status         string     `json:"status"`
		ScheduledFor   *time.Time `json:"scheduled_for"`
		ScheduledUntil *time.Time `json:"scheduled_until"`
		UpdatedAt      time.Time  `json:"updated_at"`
	} `json:"scheduled_maintenances"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
}

func (c *Client) Fetch(ctx context.Context) (pulse.Current, error) {
	data, err := c.get.Get(ctx, c.url)
	if err != nil {
		return pulse.Current{}, fmt.Errorf("current status: %w", err)
	}

	var raw response
	if err := json.Unmarshal(data, &raw); err != nil {
		return pulse.Current{}, fmt.Errorf("current status: decode JSON: %w", err)
	}
	current, err := normalize(raw)
	if err != nil {
		return pulse.Current{}, fmt.Errorf("current status: %w", err)
	}
	return current, nil
}

func normalize(raw response) (pulse.Current, error) {
	pageState, err := pulse.MapPageState(raw.Status.Indicator)
	if err != nil {
		return pulse.Current{}, err
	}
	states := []pulse.State{pageState}

	components := make([]pulse.Component, 0, len(raw.Components))
	for _, component := range raw.Components {
		state, err := pulse.MapComponentState(component.Status)
		if err != nil {
			return pulse.Current{}, err
		}
		if component.Showcase != nil && !*component.Showcase {
			continue
		}
		states = append(states, state)
		components = append(components, pulse.Component{
			ID: component.ID, Name: component.Name, State: state,
			Description: component.Description, Position: component.Position,
			Group: component.Group, GroupID: component.GroupID,
		})
	}
	sort.SliceStable(components, func(i, j int) bool { return components[i].Position < components[j].Position })

	incidents := make([]pulse.Incident, 0, len(raw.Incidents))
	for _, incident := range raw.Incidents {
		if !validIncidentStatus(incident.Status) {
			return pulse.Current{}, fmt.Errorf("unsupported incident status %q", incident.Status)
		}
		state, err := pulse.MapPageState(incident.Impact)
		if err != nil {
			return pulse.Current{}, err
		}
		var latest *pulse.IncidentUpdate
		for _, update := range incident.Updates {
			if !validIncidentStatus(update.Status) {
				return pulse.Current{}, fmt.Errorf("unsupported incident update status %q", update.Status)
			}
			if latest == nil || update.UpdatedAt.After(latest.UpdatedAt) {
				latest = &pulse.IncidentUpdate{
					Status: update.Status, Body: strings.Join(strings.Fields(update.Body), " "), UpdatedAt: update.UpdatedAt.UTC(),
				}
			}
		}
		if incident.Status == "resolved" || incident.Status == "postmortem" {
			continue
		}
		states = append(states, state)
		incidents = append(incidents, pulse.Incident{
			ID: incident.ID, Name: incident.Name, State: state, Status: incident.Status,
			UpdatedAt: incident.UpdatedAt.UTC(), LatestUpdate: latest,
		})
	}
	sort.SliceStable(incidents, func(i, j int) bool { return incidents[i].UpdatedAt.After(incidents[j].UpdatedAt) })

	maintenances := make([]pulse.MaintenanceWindow, 0, len(raw.Maintenances))
	for _, maintenance := range raw.Maintenances {
		if !validMaintenanceStatus(maintenance.Status) {
			return pulse.Current{}, fmt.Errorf("unsupported maintenance status %q", maintenance.Status)
		}
		if maintenance.Status != "in_progress" {
			continue
		}
		states = append(states, pulse.Maintenance)
		maintenances = append(maintenances, pulse.MaintenanceWindow{
			ID: maintenance.ID, Name: maintenance.Name, State: pulse.Maintenance,
			Status: maintenance.Status, ScheduledFor: utcPtr(maintenance.ScheduledFor),
			ScheduledUntil: utcPtr(maintenance.ScheduledUntil), UpdatedAt: maintenance.UpdatedAt.UTC(),
		})
	}
	sort.SliceStable(maintenances, func(i, j int) bool {
		left, right := maintenances[i].ScheduledFor, maintenances[j].ScheduledFor
		if left == nil {
			return false
		}
		return right == nil || left.Before(*right)
	})

	overallState := pulse.WorstState(states...)
	description := raw.Status.Description
	if overallState != pageState {
		description = stateDescription(overallState)
	}
	return pulse.Current{
		Overall:    pulse.Overall{State: overallState, Description: description, UpdatedAt: nonZeroTimePtr(raw.Page.UpdatedAt)},
		Components: components, ActiveIncidents: incidents, ActiveMaintenances: maintenances,
	}, nil
}

func nonZeroTimePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func validIncidentStatus(status string) bool {
	switch status {
	case "investigating", "identified", "monitoring", "resolved", "postmortem":
		return true
	default:
		return false
	}
}

func validMaintenanceStatus(status string) bool {
	switch status {
	case "scheduled", "in_progress", "verifying", "completed":
		return true
	default:
		return false
	}
}

func utcPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func stateDescription(state pulse.State) string {
	switch state {
	case pulse.Maintenance:
		return "Scheduled maintenance in progress"
	case pulse.Minor:
		return "Minor service degradation"
	case pulse.Major:
		return "Major service disruption"
	case pulse.Critical:
		return "Critical service outage"
	default:
		return "GitHub status unavailable"
	}
}
