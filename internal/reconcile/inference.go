// Package reconcile drives desired harness config to actual gateway state
// through the openshell firewall. It is SDK-free and cobra-free: it speaks only
// the openshell vocabulary, config, and the shared plan diff rule, so the write
// path and the read-only plan can never disagree on what a change is.
package reconcile

import (
	"context"
	"fmt"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// InferenceResult reports what ReconcileInference did and the resulting route.
//
// On create/update, Route is the gateway's authoritative response (Version and
// all metadata populated). On noop, Route is a partial echo of the current route
// read at plan time — Provider/Model/Route/TimeoutSecs only, no Version or
// validation metadata (the plan read does not carry them). Do not compare
// Route.Version across actions.
type InferenceResult struct {
	Action plan.Action              // create / update / noop
	Route  openshell.InferenceRoute // the route now on the gateway
}

// ReconcileInference drives the gateway's inference route to match desired. It
// reads the current route, computes the action via the shared plan.InferenceAction
// rule, and writes only when the action is create or update. Unlike the read-only
// plan it does not degrade: a transient/permission/unsupported read error is
// returned, so the caller learns the write did not happen.
//
// Precondition: desired must be Resolve-validated (config.Resolve) so its timeout
// parses; ReconcileInference re-parses defensively and errors if it does not. It
// does not gate on inference being configured (that is the plan's job via
// isInferenceConfigured) — a caller that passes an empty desired will attempt a
// create with empty provider/model and get openshell.ErrInvalidArgument.
//
// Verify is not part of the diff (the gateway does not report validation intent),
// so a config that only flips verify with provider/model/timeout unchanged yields
// noop and the new verify value does not take effect until some other field also
// changes. Likewise, an update triggered by a provider/model change with an unset
// timeout writes 0, resetting any non-default gateway timeout to the default —
// "unset timeout" always means "let the gateway decide".
func ReconcileInference(ctx context.Context, c openshell.InferenceReconciler, desired config.Inference) (InferenceResult, error) {
	cur, err := plan.ReadInferenceState(ctx, c, desired)
	if err != nil {
		return InferenceResult{}, fmt.Errorf("reading inference route: %w", err)
	}

	action := plan.InferenceAction(desired, cur)
	switch action {
	case plan.ActionValidate:
		// InferenceAction only yields validate when the gateway does not serve
		// inference route state. There is nothing to write.
		return InferenceResult{}, fmt.Errorf("inference route configuration: %w", openshell.ErrUnsupported)

	case plan.ActionNoop:
		return InferenceResult{
			Action: plan.ActionNoop,
			Route: openshell.InferenceRoute{
				Provider:    cur.Provider,
				Model:       cur.Model,
				Route:       cur.Route,
				TimeoutSecs: cur.TimeoutSecs,
			},
		}, nil

	case plan.ActionCreate, plan.ActionUpdate:
		secs, err := desired.TimeoutSecs()
		if err != nil {
			return InferenceResult{}, fmt.Errorf("inference timeout: %w", err)
		}
		// The single site mapping positive-sense Verify to the SDK's negative
		// NoVerify. SetInferenceRoute is an upsert, so it serves create and update.
		route, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
			Provider:    desired.Provider,
			Model:       desired.Model,
			Route:       plan.ResolveInferenceRoute(desired.Route),
			NoVerify:    !desired.VerifyEnabled(),
			TimeoutSecs: secs,
		})
		if err != nil {
			return InferenceResult{}, fmt.Errorf("setting inference route: %w", err)
		}
		return InferenceResult{Action: action, Route: route}, nil

	default:
		return InferenceResult{}, fmt.Errorf("unexpected inference action %q", action)
	}
}
