package cmd

import (
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

func TestActiveGatewayInfo_ListError(t *testing.T) {
	gw := &mockGW{}

	_, err := activeGatewayInfo(gw)
	if err == nil {
		t.Fatal("expected error when no active gateway")
	}
	if !strings.Contains(err.Error(), "no active gateway") {
		t.Errorf("error = %q, want 'no active gateway'", err)
	}
}

func TestActiveGatewayInfo_RemoteGateway(t *testing.T) {
	gw := &mockGW{
		gatewayListResult: []gateway.GatewayInfo{
			{Name: "openshell-remote-ocp", Endpoint: "https://gateway.apps.ocp.example.com:443", Active: true},
		},
	}

	info, err := activeGatewayInfo(gw)
	if err != nil {
		t.Fatalf("activeGatewayInfo: %v", err)
	}
	if info.Name != "openshell-remote-ocp" {
		t.Errorf("Name = %q, want openshell-remote-ocp", info.Name)
	}
}


