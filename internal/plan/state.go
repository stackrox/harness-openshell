package plan

import (
	"context"
	"errors"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// InferenceState captures the gateway's reported inference capabilities.
type InferenceState struct {
	Capable bool   // whether the gateway reports its inference route
	Route   string // the reported route, set only when Capable is true
}

// CurrentState is a snapshot of the gateway's current state, read at plan time.
type CurrentState struct {
	Health    openshell.Health
	Reachable bool
	Providers []openshell.Provider
	Inference InferenceState
}

// ReadCurrentState reads the current gateway state and returns a snapshot. It is
// the only I/O in the package. It degrades gracefully: if the gateway is
// unreachable or unauthenticated, Reachable is set to false and a nil error is
// returned; other errors are escalated. Inference.Capable is left false because
// the gateway does not report its inference route.
func ReadCurrentState(ctx context.Context, c openshell.Client, desired *config.Harness) (CurrentState, error) {
	var state CurrentState

	// Read health.
	health, err := c.Health(ctx)
	if err != nil {
		if errors.Is(err, openshell.ErrUnavailable) || errors.Is(err, openshell.ErrUnauthenticated) {
			state.Reachable = false
			return state, nil
		}
		return state, err
	}
	state.Health = health
	state.Reachable = health.Healthy

	// Read providers.
	providers, err := c.Providers(ctx)
	if err != nil {
		if errors.Is(err, openshell.ErrUnavailable) || errors.Is(err, openshell.ErrUnauthenticated) {
			state.Reachable = false
			return state, nil
		}
		return state, err
	}
	state.Providers = providers

	return state, nil
}
