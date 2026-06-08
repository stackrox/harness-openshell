package cmd

import (
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/preflight"
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

func TestProfileHasCustomProviders_NoCustom(t *testing.T) {
	allProviders := []preflight.Provider{
		{Name: "github", Type: "openshell"},
		{Name: "vertex-local", Type: "openshell"},
	}
	if profileHasCustomProviders([]string{"github", "vertex-local"}, allProviders) {
		t.Error("no custom providers, should return false")
	}
}

func TestProfileHasCustomProviders_WithCustom(t *testing.T) {
	allProviders := []preflight.Provider{
		{Name: "github", Type: "openshell"},
		{Name: "gws", Type: "custom"},
	}
	if !profileHasCustomProviders([]string{"github", "gws"}, allProviders) {
		t.Error("gws is custom, should return true")
	}
}

func TestProviderInList(t *testing.T) {
	if !providerInList("github", []string{"github", "vertex-local"}) {
		t.Error("github should be in list")
	}
	if providerInList("atlassian", []string{"github", "vertex-local"}) {
		t.Error("atlassian should not be in list")
	}
	if providerInList("github", nil) {
		t.Error("nil list should return false")
	}
}
