package run

import (
	"context"
	"time"

	"github.com/stackrox/harness-openshell/internal/gateway"
)

// SandboxRunRequest carries the neutral vocabulary needed to create and run a
// sandbox. All fields are primitives — no agent, cobra, or SDK types — allowing
// this package to remain firewall-clean (invariant 32).
type SandboxRunRequest struct {
	// Name is the sandbox name.
	Name string

	// Gateway is the gateway context name (empty → active context).
	Gateway string

	// Workspace is the workspace name (empty → default).
	Workspace string

	// Image is the sandbox image ref or relative Dockerfile dir.
	Image string

	// Providers are the registered providers to attach, in declared order.
	Providers []string

	// NoAutoProviders, when true, disables auto-discovery of providers.
	NoAutoProviders bool

	// Env is the environment variables to inject via --env on sandbox create.
	Env map[string]string

	// Command is the argv to execute inside the sandbox (adapter-produced).
	Command []string

	// Uploads are additional uploads to stage in the sandbox (caller pre-stages
	// payload dir; S5 owns temp dirs).
	Uploads []gateway.Upload

	// TTY, when true, preserves native TTY streaming (invariant 31).
	TTY bool

	// Keep, when true, retains the sandbox after creation; when false, deletes it.
	Keep bool

	// PolicyPath is the staged policy file path ("" → omit --policy).
	PolicyPath string

	// Labels are arbitrary key-value labels to attach to the sandbox.
	Labels map[string]string

	// RetrySleep is the duration to pause between retry attempts. Zero means no
	// pause (tests pass zero; callers pass the real backoff).
	RetrySleep time.Duration
}

// SandboxRunner is the minimal interface required to execute a sandbox lifecycle.
// The real *gateway.CLI and gateway.Gateway satisfy this structurally.
type SandboxRunner interface {
	SandboxCreate(opts gateway.SandboxCreateOpts) error
	SandboxDelete(name string) error
}

// RunSandbox creates and runs a sandbox with the given request, mirroring the
// behavior of cmd/sandbox.go's createSandbox. It is the single owner of sandbox
// execution (invariant 27).
//
// Behavior:
//   - Maps req → gateway.SandboxCreateOpts field-for-field.
//   - Bounded retry: up to 5 attempts. On each failure, attempts best-effort
//     SandboxDelete and then retries (sleeping between attempts).
//   - After the last failure, returns a wrapped error mentioning the attempt count.
//   - Honors context cancellation: if ctx.Err() != nil, returns immediately
//     without calling SandboxCreate.
//   - Preserves native TTY (does not wrap stdout/stderr).
//   - On success, the sandbox is retained or deleted according to Keep.
func RunSandbox(ctx context.Context, gw SandboxRunner, req SandboxRunRequest) error {
	return runSandboxWithLifecycle(ctx, gw, req)
}
