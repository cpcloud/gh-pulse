package pulse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapPageStateRejectsUnknownInsteadOfLookingHealthy(t *testing.T) {
	t.Parallel()

	_, err := MapPageState("surprising")
	require.Error(t, err)
}

func TestMapPageStatePreservesSeverity(t *testing.T) {
	t.Parallel()

	tests := map[string]State{
		"none":        Operational,
		"maintenance": Maintenance,
		"minor":       Minor,
		"major":       Major,
		"critical":    Critical,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := MapPageState(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestMapComponentStatePreservesSeverity(t *testing.T) {
	t.Parallel()

	tests := map[string]State{
		"operational":          Operational,
		"under_maintenance":    Maintenance,
		"degraded_performance": Minor,
		"partial_outage":       Major,
		"major_outage":         Critical,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			got, err := MapComponentState(input)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestMapComponentStateRejectsUnknownInsteadOfLookingHealthy(t *testing.T) {
	t.Parallel()

	_, err := MapComponentState("surprising")
	require.Error(t, err)
}

func TestWorstStateDoesNotLetMaintenanceHideAnIncident(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Major, WorstState(Operational, Maintenance, Minor, Major))
}

func TestWorstStateIgnoresUnknownForPrecedence(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Minor, WorstState(Unknown, Operational, Minor))
}
