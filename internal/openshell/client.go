// Package openshell is the harness-owned firewall over the OpenShell Go SDK.
//
// It defines the vocabulary the rest of harness-openshell uses to talk to a
// gateway — Client, Target, Health, Provider, and the sentinel errors — without
// leaking any SDK type. This package MUST NOT import the OpenShell SDK; the only
// package permitted to translate between these types and the SDK is
// internal/openshell/sdkclient (production) and internal/testutil (tests).
package openshell

import "context"

// Client is the harness-owned view of a single (gateway, workspace) target.
//
// The workspace is bound at construction, so methods never take one — a Client
// speaks for exactly one workspace on one gateway.
type Client interface {
	// Health reports whether the gateway is reachable and healthy.
	Health(ctx context.Context) (Health, error)
	// Providers lists the providers registered in the bound workspace.
	Providers(ctx context.Context) ([]Provider, error)
	// Close releases any resources held by the client.
	Close() error
}

// Factory constructs a Client for a Target.
//
// This is the only construction seam commands are allowed to depend on:
// production wires sdkclient.New, tests wire testutil.FakeFactory. No command
// calls into the SDK — or into sdkclient — directly.
type Factory func(ctx context.Context, t Target) (Client, error)
