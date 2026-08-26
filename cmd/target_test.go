package cmd

import (
	"strings"
	"testing"
)

func TestResolveApplyTarget_FromActiveGateway(t *testing.T) {
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

func TestResolveApplyTarget_EmptyActiveGatewayErrors(t *testing.T) {
	gw := &mockGW{activeGateway: ""}

	_, err := resolveApplyTarget(gw)
	if err == nil {
		t.Fatal("expected error for empty active gateway, got nil")
	}
	if !strings.Contains(err.Error(), "no active openshell gateway") {
		t.Errorf("error = %q, want mention of no active gateway", err)
	}
}
