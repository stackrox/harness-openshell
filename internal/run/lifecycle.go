package run

import (
	"context"
	"fmt"
	"time"

	"github.com/stackrox/harness-openshell/internal/gateway"
)

const maxRetries = 5

// toCreateOpts maps a SandboxRunRequest to gateway.SandboxCreateOpts.
func toCreateOpts(req SandboxRunRequest) gateway.SandboxCreateOpts {
	opts := gateway.SandboxCreateOpts{
		Name:            req.Name,
		From:            req.Image,
		Providers:       req.Providers,
		NoAutoProviders: req.NoAutoProviders,
		TTY:             req.TTY,
		Keep:            req.Keep,
		Uploads:         req.Uploads,
		Command:         req.Command,
		Env:             req.Env,
		Gateway:         req.Gateway,
		Workspace:       req.Workspace,
		Labels:          req.Labels,
	}
	if req.PolicyPath != "" {
		opts.Policy = req.PolicyPath
	}
	return opts
}

// runSandboxWithLifecycle implements the bounded-retry loop with best-effort
// cleanup (invariant 34). It mirrors cmd/sandbox.go's createSandbox exactly.
func runSandboxWithLifecycle(ctx context.Context, gw SandboxRunner, req SandboxRunRequest) error {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Honor context cancellation before each attempt.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		opts := toCreateOpts(req)
		err := gw.SandboxCreate(opts)
		if err == nil {
			// Success; sandbox was created and is retained per Keep.
			return nil
		}

		// Failed; attempt best-effort cleanup (ignore its error).
		gw.SandboxDelete(req.Name)

		// On the last attempt, return the wrapped error.
		if attempt == maxRetries {
			return fmt.Errorf("sandbox create failed after %d attempts: %w", maxRetries, err)
		}

		// Pause before the next attempt, honoring cancellation.
		if req.RetrySleep > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(req.RetrySleep):
			}
		}
	}
	return nil // unreachable: the loop returns on attempt == maxRetries
}
