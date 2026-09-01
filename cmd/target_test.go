package cmd

import (
	"testing"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// With neither --gateway nor $OPENSHELL_GATEWAY set, the resolved target's
// Gateway stays empty. Active-gateway resolution is no longer done here: the SDK
// (gateway.LoadConfig("") in sdkclient.New) owns it, surfacing ErrNoActiveGateway
// if none is selected.
func TestResolveApplyTarget_EmptyWhenUnset(t *testing.T) {
	t.Setenv(openshell.EnvGateway, "")
	t.Setenv(openshell.EnvWorkspace, "")

	target := resolveApplyTarget("", "")
	if target.Gateway != "" {
		t.Errorf("Gateway = %q, want empty (SDK resolves the active gateway)", target.Gateway)
	}
	if target.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (sdkclient defaults it)", target.Workspace)
	}
}

func TestResolveApplyTarget_FlagsOverrideEnvironment(t *testing.T) {
	t.Setenv(openshell.EnvGateway, "env-gw")
	t.Setenv(openshell.EnvWorkspace, "env-ws")

	target := resolveApplyTarget("flag-gw", "flag-ws")
	if target != (openshell.Target{Gateway: "flag-gw", Workspace: "flag-ws"}) {
		t.Errorf("target = %+v", target)
	}
}

// $OPENSHELL_GATEWAY targets a gateway without an explicit flag.
func TestResolveApplyTarget_EnvGateway(t *testing.T) {
	t.Setenv(openshell.EnvGateway, "env-gw")

	target := resolveApplyTarget("", "")
	if target.Gateway != "env-gw" {
		t.Errorf("Gateway = %q, want env-gw", target.Gateway)
	}
}
