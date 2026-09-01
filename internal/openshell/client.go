// Package openshell is the harness-owned firewall over the OpenShell Go SDK.
//
// It defines the vocabulary the rest of harness-openshell uses to talk to a
// gateway — Client, Target, Health, Provider, and the sentinel errors — without
// leaking any SDK type. This package MUST NOT import the OpenShell SDK; the only
// package permitted to translate between these types and the SDK is
// internal/openshell/sdkclient (production) and internal/testutil (tests).
package openshell

import (
	"context"
	"io"
)

// Client is the harness-owned view of a single (gateway, workspace) target.
//
// The workspace is bound at construction, so methods never take one — a Client
// speaks for exactly one workspace on one gateway.
type Client interface {
	// Health reports whether the gateway is reachable and healthy.
	Health(ctx context.Context) (Health, error)
	// Providers lists the providers registered in the bound workspace.
	Providers(ctx context.Context) ([]Provider, error)
	// Sandboxes lists the sandboxes in the bound workspace (read UX: get agents).
	Sandboxes(ctx context.Context) ([]Sandbox, error)
	// GetSandbox reads the named sandbox in the bound workspace. Returns
	// ErrNotFound when no such sandbox exists (read UX: describe).
	GetSandbox(ctx context.Context, name string) (Sandbox, error)
	// DeleteSandbox removes the named sandbox in the bound workspace.
	DeleteSandbox(ctx context.Context, name string) error
	// DeleteProvider removes the named provider in the bound workspace.
	DeleteProvider(ctx context.Context, name string) error
	// GatewayInfo introspects the active gateway (name, endpoint, status,
	// version). The SDK offers no gateway list; this reports the single gateway
	// the client is bound to.
	GatewayInfo(ctx context.Context) (GatewayInfo, error)
	// GetProvider reads the named provider in the bound workspace. Returns
	// ErrNotFound when no such provider exists (requires the "provider:read"
	// role).
	GetProvider(ctx context.Context, name string) (Provider, error)
	// UpdateProvider writes the desired non-secret Config, Labels, and Type of an
	// existing provider, preserving its stored credentials. It is
	// credential-preserving by construction: the harness Provider carries no
	// credentials, and sdkclient overlays only those non-secret fields onto the
	// provider's current server object (see sdkclient.UpdateProvider). Reconcile
	// issues it only on a real non-secret delta. Requires the workspace "admin" role plus
	// "provider:write"; a caller lacking either gets ErrPermission.
	UpdateProvider(ctx context.Context, p Provider) (Provider, error)
	// GetInferenceRoute reads the named inference route in the bound workspace.
	// An empty route targets the gateway default route. Returns ErrNotFound when
	// no such route exists (requires the workspace "user" role).
	GetInferenceRoute(ctx context.Context, route string) (InferenceRoute, error)
	// SetInferenceRoute creates or updates an inference route in the bound
	// workspace (upsert). Returns the resulting route. Requires the workspace
	// "admin" role; a caller lacking it gets ErrPermission.
	SetInferenceRoute(ctx context.Context, cfg InferenceRouteConfig) (InferenceRoute, error)
	// DeleteInferenceRoute removes the named inference route in the bound
	// workspace. Idempotent: deleting a missing route is not an error. Requires
	// the workspace "admin" role.
	DeleteInferenceRoute(ctx context.Context, route string) error
	// Close releases any resources held by the client.
	Close() error
}

// SandboxExecutionClient is the narrow SDK-native execution surface. It is
// separate from Client so read/reconcile fakes do not need to implement runtime
// operations they never use.
type SandboxExecutionClient interface {
	CreateSandbox(ctx context.Context, desired SandboxCreate) (Sandbox, error)
	WaitSandboxReady(ctx context.Context, name string) (Sandbox, error)
	ExecSandbox(ctx context.Context, name string, command []string, stdout, stderr io.Writer) (int, error)
	ExecInteractive(ctx context.Context, name string, command []string, cols, rows uint32) (InteractiveSession, error)
	DeleteSandbox(ctx context.Context, name string) error
}

// InteractiveSession is the SDK-native bidirectional terminal stream without
// exposing an SDK type outside sdkclient.
type InteractiveSession interface {
	io.Reader
	io.Writer
	Resize(cols, rows uint32) error
	ExitCode() (int, error)
	Close() error
}

// Factory constructs a Client for a Target.
//
// This is the only construction seam commands are allowed to depend on:
// production wires sdkclient.New, tests wire testutil.FakeFactory. No command
// calls into the SDK — or into sdkclient — directly.
type Factory func(ctx context.Context, t Target) (Client, error)
