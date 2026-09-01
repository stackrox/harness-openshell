package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// registerTargetFlags adds the standard --gateway/--workspace flags to cmd and
// returns pointers to their values. Every SDK-backed command registers its
// target flags through this one helper so the flag names, help text, and the
// OPENSHELL_* env fallbacks stay identical across commands. The flags are
// per-command (not root-persistent).
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
