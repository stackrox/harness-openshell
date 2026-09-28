// Package testutil provides test utilities for exercising the harness layer
// against the OpenShell Go SDK fake, which validates the real sdkclient
// mapping/translation without hitting a live gateway.
package testutil

import (
	"context"
	"fmt"
	"sync"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"

	"github.com/stackrox/harness-openshell/runner/internal/openshell"
	"github.com/stackrox/harness-openshell/runner/internal/openshell/sdkclient"
)

// NewFake returns an openshell.Client backed by the SDK fake, exercising the
// real sdkclient mapping/translation for v0.1.2-supported resources. Legacy
// inference-route tests use the in-memory compatibility seam below. Seed the
// fake via fake.With* options before construction. For tests that need to call
// fake.Client.AddProvider after construction, use NewFakeClient instead.
func NewFake(workspace string, opts ...fake.ClientOption) openshell.Client {
	c, _ := NewFakeClient(workspace, opts...)
	return c
}

// NewFakeClient returns an openshell.Client backed by the SDK fake and the
// underlying *fake.Client for direct test manipulation. This allows tests to
// call fake.Client.AddProvider on the returned *fake.Client after construction.
// Implement NewFake in terms of this to avoid duplication.
func NewFakeClient(workspace string, opts ...fake.ClientOption) (openshell.Client, *fake.Client) {
	raw := fake.NewClient(opts...)
	return &fakeClient{
		Client: sdkclient.NewFromClient(raw, workspace),
		routes: make(map[string]openshell.InferenceRoute),
	}, raw
}

// fakeClient keeps the harness's pre-v0.1 inference-route seam available to
// plan/reconcile unit tests. OpenShell v0.1.2 removed managed inference routes
// from the SDK; production sdkclient returns ErrUnsupported instead.
type fakeClient struct {
	openshell.Client
	mu      sync.Mutex
	routes  map[string]openshell.InferenceRoute
	version uint64
}

func (c *fakeClient) GetInferenceRoute(_ context.Context, route string) (openshell.InferenceRoute, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.routes[route]
	if !ok {
		return openshell.InferenceRoute{}, fmt.Errorf("%w: inference route %q", openshell.ErrNotFound, route)
	}
	return r, nil
}

func (c *fakeClient) SetInferenceRoute(_ context.Context, cfg openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cfg.Provider == "" || cfg.Model == "" {
		return openshell.InferenceRoute{}, fmt.Errorf("%w: provider and model are required", openshell.ErrInvalidArgument)
	}
	c.version++
	route := cfg.Route
	c.routes[route] = openshell.InferenceRoute{
		Provider:    cfg.Provider,
		Model:       cfg.Model,
		Route:       route,
		TimeoutSecs: cfg.TimeoutSecs,
		Version:     c.version,
	}
	return c.routes[route], nil
}

func (c *fakeClient) DeleteInferenceRoute(_ context.Context, route string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.routes, route)
	return nil
}

// FakeFactory returns a Factory closure that ignores its context and Target
// arguments and always returns the given Client and nil error. Use this to
// wire a test client into code that depends on the Factory seam.
func FakeFactory(c openshell.Client) openshell.Factory {
	return func(context.Context, openshell.Target) (openshell.Client, error) {
		return c, nil
	}
}
