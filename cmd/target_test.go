package cmd

import (
	"strings"
	"testing"
)

func TestResolveApplyTarget_FromActiveGateway(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "") // isolate from the caller's environment
	gw := &mockGW{activeGateway: "prod-gw"}

	target, err := resolveApplyTarget(gw)
	if err != nil {
		t.Fatalf("resolveApplyTarget: %v", err)
	}
	if target.Gateway != "prod-gw" {
		t.Errorf("Gateway = %q, want prod-gw", target.Gateway)
	}
	if target.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (sdkclient defaults it)", target.Workspace)
	}
}

// $OPENSHELL_GATEWAY changes OpenShell's request target without moving the
// active-gateway marker, so apply must honor it over ActiveGateway().
func TestResolveApplyTarget_EnvOverridesActiveGateway(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "env-gw")
	gw := &mockGW{activeGateway: "active-gw"}

	target, err := resolveApplyTarget(gw)
	if err != nil {
		t.Fatalf("resolveApplyTarget: %v", err)
	}
	if target.Gateway != "env-gw" {
		t.Errorf("Gateway = %q, want env-gw ($OPENSHELL_GATEWAY overrides active)", target.Gateway)
	}
}

// With no active-gateway marker, $OPENSHELL_GATEWAY alone is enough to target.
func TestResolveApplyTarget_EnvWithNoActiveGateway(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "env-gw")
	gw := &mockGW{activeGateway: ""}

	target, err := resolveApplyTarget(gw)
	if err != nil {
		t.Fatalf("resolveApplyTarget: %v", err)
	}
	if target.Gateway != "env-gw" {
		t.Errorf("Gateway = %q, want env-gw", target.Gateway)
	}
}

func TestResolveApplyTarget_EmptyActiveGatewayErrors(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY", "") // no env override, no active gateway
	gw := &mockGW{activeGateway: ""}

	_, err := resolveApplyTarget(gw)
	if err == nil {
		t.Fatal("expected error for empty active gateway, got nil")
	}
	if !strings.Contains(err.Error(), "no active openshell gateway") {
		t.Errorf("error = %q, want mention of no active gateway", err)
	}
}
