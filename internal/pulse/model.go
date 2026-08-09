package pulse

import (
	"fmt"
	"time"
)

// State is Pulse's source-independent health vocabulary.
type State string

const (
	Unknown     State = "unknown"
	Operational State = "operational"
	Maintenance State = "maintenance"
	Minor       State = "minor"
	Major       State = "major"
	Critical    State = "critical"
)

var ranks = map[State]int{
	Operational: 0,
	Maintenance: 1,
	Minor:       2,
	Major:       3,
	Critical:    4,
}

func MapPageState(value string) (State, error) {
	switch value {
	case "none":
		return Operational, nil
	case "maintenance":
		return Maintenance, nil
	case "minor":
		return Minor, nil
	case "major":
		return Major, nil
	case "critical":
		return Critical, nil
	default:
		return Unknown, fmt.Errorf("unsupported status impact %q", value)
	}
}

func MapComponentState(value string) (State, error) {
	switch value {
	case "operational":
		return Operational, nil
	case "under_maintenance":
		return Maintenance, nil
	case "degraded_performance":
		return Minor, nil
	case "partial_outage":
		return Major, nil
	case "major_outage":
		return Critical, nil
	default:
		return Unknown, fmt.Errorf("unsupported component status %q", value)
	}
}

// WorstState compares already validated states. Unknown represents unavailable
// source data and deliberately does not participate in severity precedence.
func WorstState(states ...State) State {
	worst := Unknown
	worstRank := -1
	for _, state := range states {
		rank, ok := ranks[state]
		if ok && rank > worstRank {
			worst = state
			worstRank = rank
		}
	}
	return worst
}

type Overall struct {
	State       State      `json:"state"`
	Description string     `json:"description"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

type Component struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	State       State   `json:"state"`
	Description *string `json:"description"`
	Position    int     `json:"position"`
	Group       bool    `json:"group"`
	GroupID     *string `json:"group_id"`
}

type IncidentUpdate struct {
	Status    string    `json:"status"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Incident struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	State        State           `json:"state"`
	Status       string          `json:"status"`
	UpdatedAt    time.Time       `json:"updated_at"`
	LatestUpdate *IncidentUpdate `json:"latest_update"`
}

type MaintenanceWindow struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	State          State      `json:"state"`
	Status         string     `json:"status"`
	ScheduledFor   *time.Time `json:"scheduled_for"`
	ScheduledUntil *time.Time `json:"scheduled_until"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Current struct {
	Overall            Overall
	Components         []Component
	ActiveIncidents    []Incident
	ActiveMaintenances []MaintenanceWindow
}

type FeedEntry struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	URL         *string    `json:"url"`
	PublishedAt *time.Time `json:"published_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Feed struct {
	UpdatedAt time.Time
	Entries   []FeedEntry
}

type Day struct {
	Date  string `json:"date"`
	State State  `json:"state"`
}

type RollingPoint struct {
	Date   string  `json:"date"`
	Uptime float64 `json:"uptime"`
}

type RollingHistory struct {
	Current *RollingPoint  `json:"current"`
	Best    *RollingPoint  `json:"best"`
	Worst   *RollingPoint  `json:"worst"`
	Series  []RollingPoint `json:"series"`
}

type ComponentHistory struct {
	Name         string   `json:"name"`
	Days90       []Day    `json:"days_90"`
	Uptime90Days *float64 `json:"uptime_90_days"`
}

type History struct {
	Source         string             `json:"source"`
	CoverageStart  time.Time          `json:"coverage_start"`
	AsOf           time.Time          `json:"as_of"`
	Days90         []Day              `json:"days_90"`
	Uptime90Days   *float64           `json:"uptime_90_days"`
	Downtime90Days *float64           `json:"downtime_90_days"`
	TrackedUptime  *float64           `json:"tracked_uptime"`
	Rolling90Days  RollingHistory     `json:"rolling_90_days"`
	Components     []ComponentHistory `json:"components"`
}

type Source struct {
	URL       string     `json:"url"`
	Available bool       `json:"available"`
	FetchedAt *time.Time `json:"fetched_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type Sources struct {
	Current Source `json:"current"`
	Feed    Source `json:"feed"`
	History Source `json:"history"`
}

type SourceError struct {
	Source      string     `json:"source"`
	Message     string     `json:"message"`
	AttemptedAt *time.Time `json:"-"`
}

type Snapshot struct {
	SchemaVersion      int                 `json:"schema_version"`
	GeneratedAt        time.Time           `json:"generated_at"`
	Overall            Overall             `json:"overall"`
	Components         []Component         `json:"components"`
	ActiveIncidents    []Incident          `json:"active_incidents"`
	ActiveMaintenances []MaintenanceWindow `json:"active_maintenances"`
	RecentFeed         []FeedEntry         `json:"recent_feed"`
	History            History             `json:"history"`
	Sources            Sources             `json:"sources"`
	Errors             []SourceError       `json:"errors"`
}
