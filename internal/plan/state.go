package plan

import (
	"context"
	"errors"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// DefaultInferenceRoute is the route name the gateway assigns when a config
// leaves spec.inference.route empty. The harness resolves "" to this name so the
// plan read and the reconcile write address the same route (the SDK fake does
// not default an empty name; a real gateway does). Single owner: reconcile reads
// it from here rather than redefining it.
const DefaultInferenceRoute = "inference.local"

// InferenceState is the gateway's current inference route, read at plan time.
//
// Capable reports whether the gateway serves inference route state at all: an
// older gateway that does not implement the RPC leaves Capable false and the
// plan falls back to a config-only validate. When Capable is true, Present says
// whether a route exists, and the remaining fields carry it for the diff.
type InferenceState struct {
	Capable     bool   // the gateway serves inference route state
	Present     bool   // a route exists (only meaningful when Capable)
	Provider    string // populated when Present
	Model       string // populated when Present
	Route       string // populated when Present
	TimeoutSecs uint64 // populated when Present
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
// returned; other errors are escalated. When desired configures inference, it
// reads the current route so the plan can show a real create/update/noop diff.
func ReadCurrentState(ctx context.Context, c openshell.StateReader, desired *config.Harness) (CurrentState, error) {
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

	// Read the inference route, but only when desired configures inference (no
	// point probing an unused subsystem). Health and providers already proved the
	// gateway reachable, so a transient inference-read failure must not flip
	// Reachable (that would render a misleading login-required target next to
	// populated providers). Instead it degrades to the not-capable validate
	// fallback — the same config-only outcome as an older gateway that does not
	// serve inference state at all.
	if isInferenceConfigured(desired.Spec.Inference) {
		inf, err := ReadInferenceState(ctx, c, desired.Spec.Inference)
		if err != nil {
			if errors.Is(err, openshell.ErrUnavailable) || errors.Is(err, openshell.ErrUnauthenticated) {
				state.Inference = InferenceState{Capable: false}
			} else {
				return state, err
			}
		} else {
			state.Inference = inf
		}
	}

	return state, nil
}

// ReadInferenceState reads the current inference route for the desired config.
// An absent route is not an error (Capable, not Present); a gateway that does
// not serve inference (ErrUnsupported) leaves Capable false so the plan falls
// back to a config-only validate. Transient errors (unavailable/unauthenticated)
// and ErrPermission are propagated so the caller decides whether to degrade
// (the read-only plan) or fail (the reconcile write path).
func ReadInferenceState(ctx context.Context, c openshell.InferenceRouteReader, desired config.Inference) (InferenceState, error) {
	route, err := c.GetInferenceRoute(ctx, ResolveInferenceRoute(desired.Route))
	switch {
	case err == nil:
		return InferenceState{
			Capable:     true,
			Present:     true,
			Provider:    route.Provider,
			Model:       route.Model,
			Route:       route.Route,
			TimeoutSecs: route.TimeoutSecs,
		}, nil
	case errors.Is(err, openshell.ErrNotFound):
		return InferenceState{Capable: true, Present: false}, nil
	case errors.Is(err, openshell.ErrUnsupported):
		return InferenceState{Capable: false}, nil
	default:
		return InferenceState{}, err
	}
}

// ResolveInferenceRoute maps an empty configured route to the gateway default so
// the read and the write address the same route.
func ResolveInferenceRoute(route string) string {
	if route == "" {
		return DefaultInferenceRoute
	}
	return route
}
