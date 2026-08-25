package sdkclient

import (
	"context"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKInferenceRoute maps the SDK route view to the minimal harness view.
// Deliberately narrow (least-exposure firewall); widen only when a consumer
// genuinely needs more fields, changing this and openshell.InferenceRoute
// together.
func fromSDKInferenceRoute(r *v1.InferenceRoute) openshell.InferenceRoute {
	return openshell.InferenceRoute{
		Provider:    r.ProviderName,
		Model:       r.ModelID,
		Route:       r.RouteName,
		TimeoutSecs: r.TimeoutSecs,
		Version:     r.Version,
	}
}

// GetInferenceRoute reads the named inference route in the bound workspace.
func (c *client) GetInferenceRoute(ctx context.Context, route string) (openshell.InferenceRoute, error) {
	r, err := c.raw.Inference().GetRoute(ctx, c.workspace, route)
	if err != nil {
		return openshell.InferenceRoute{}, translate(err)
	}
	return fromSDKInferenceRoute(r), nil
}

// SetInferenceRoute creates or updates (upserts) an inference route in the bound
// workspace.
func (c *client) SetInferenceRoute(ctx context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	r, err := c.raw.Inference().SetRoute(ctx, c.workspace, &v1.InferenceRouteConfig{
		ProviderName: cfg.Provider,
		ModelID:      cfg.Model,
		RouteName:    cfg.Route,
		NoVerify:     cfg.NoVerify,
		TimeoutSecs:  cfg.TimeoutSecs,
	})
	if err != nil {
		return openshell.InferenceRoute{}, translate(err)
	}
	return fromSDKInferenceRoute(r), nil
}

// DeleteInferenceRoute removes the named inference route in the bound workspace.
func (c *client) DeleteInferenceRoute(ctx context.Context, route string) error {
	return translate(c.raw.Inference().DeleteRoute(ctx, c.workspace, route))
}
