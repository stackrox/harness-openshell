package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// registerTargetFlags adds the standard --gateway/--workspace flags to cmd and
// returns pointers to their values. Every SDK-backed command registers its
// target flags through this one helper so the flag names, help text, and the
// OPENSHELL_* env fallbacks stay identical across commands. The flags are
// per-command (not root-persistent) so legacy CLI-path commands never carry
// them.
//
// The returned pointers are meant to be fed to openshell.ResolveTarget together
// with os.Getenv; resolution (flag > env > empty) and the workspace default live
// there and in sdkclient, not here.
func registerTargetFlags(cmd *cobra.Command) (gateway, workspace *string) {
	gateway = cmd.Flags().String("gateway", "",
		fmt.Sprintf("OpenShell gateway registration name (falls back to $%s).", openshell.EnvGateway))
	workspace = cmd.Flags().String("workspace", "",
		fmt.Sprintf("OpenShell workspace (defaults to %q; falls back to $%s).", "default", openshell.EnvWorkspace))
	return gateway, workspace
}

// openClient resolves the standard --gateway/--workspace target (flag > env >
// empty, via openshell.ResolveTarget) and constructs an SDK client through the
// Factory seam. It is the shared construction site for get and describe, so
// target resolution stays identical across them. delete resolves the target
// itself (it needs the resolved gateway name for its banner) but uses the same
// ResolveTarget rule. Callers own the returned client's Close.
func openClient(ctx context.Context, newClient openshell.Factory, gatewayName, workspace *string) (openshell.Client, error) {
	target := openshell.ResolveTarget(*gatewayName, *workspace, "", "", os.Getenv)
	return newClient(ctx, target)
}

// resolveApplyTarget builds the SDK openshell.Target for the apply command from
// the CLI's currently-active gateway registration.
//
// Apply deliberately does NOT register the standard --gateway/--workspace target
// flags: the harness runs against whichever gateway OpenShell has selected, so
// the registration name is read from the active gateway the CLI already
// selected rather than pinned per-invocation.
//
// $OPENSHELL_GATEWAY takes precedence over the CLI's persisted active-gateway
// marker, matching OpenShell's own request-targeting precedence: the env var
// overrides the target without moving the `*` in `openshell gateway list`, so
// reading only ActiveGateway() would wrongly reject an env-targeted apply.
//
// An empty target is an error, not a silent skip: the harness does not provision
// gateways (that is OpenShell's job), so without a selected registration there is
// nothing to run against. Workspace is left "" so sdkclient applies its "default"
// default (the single owner of that rule).
func resolveApplyTarget(gw gateway.Gateway) (openshell.Target, error) {
	name := os.Getenv(openshell.EnvGateway)
	if name == "" {
		name = gw.ActiveGateway()
	}
	if name == "" {
		return openshell.Target{}, fmt.Errorf("no active openshell gateway — run 'openshell gateway select <name>' first (provision one with the OpenShell installer or 'helm install openshell')")
	}
	return openshell.Target{Gateway: name, Workspace: ""}, nil
}
