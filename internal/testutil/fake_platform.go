// Package testutil provides test utilities for exercising the harness layer
// against the OpenShell Go SDK fake, which validates the real sdkclient
// mapping/translation without hitting a live gateway.
package testutil

import (
	"context"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"

	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
)

// NewFake returns an openshell.Client backed by the SDK fake, exercising the
// REAL sdkclient mapping/translation. Seed the fake via fake.With* options
// before construction. For tests that need to call fake.Client.AddProvider
// after construction, use NewFakeClient instead.
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
	return sdkclient.NewFromClient(raw, workspace), raw
}

// FakeFactory returns a Factory closure that ignores its context and Target
// arguments and always returns the given Client and nil error. Use this to
// wire a test client into code that depends on the Factory seam.
func FakeFactory(c openshell.Client) openshell.Factory {
	return func(context.Context, openshell.Target) (openshell.Client, error) {
		return c, nil
	}
}
