package sdkclient

import (
	"context"
	"fmt"

	"github.com/stackrox/harness-openshell/runner/internal/openshell"
)

var errManagedInferenceRemoved = fmt.Errorf("%w: managed inference routes were removed in OpenShell v0.1.0", openshell.ErrUnsupported)

// GetInferenceRoute reads the named inference route in the bound workspace.
func (c *client) GetInferenceRoute(ctx context.Context, route string) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, errManagedInferenceRemoved
}

// SetInferenceRoute creates or updates (upserts) an inference route in the bound
// workspace.
func (c *client) SetInferenceRoute(ctx context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	return openshell.InferenceRoute{}, errManagedInferenceRemoved
}

// DeleteInferenceRoute removes the named inference route in the bound workspace.
func (c *client) DeleteInferenceRoute(ctx context.Context, route string) error {
	return errManagedInferenceRemoved
}
