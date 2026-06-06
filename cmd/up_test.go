package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/preflight"
)

func TestUpLocal_NoGateway(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{inferenceErr: fmt.Errorf("connection refused")}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no active gateway") {
		t.Errorf("error = %q, want 'no active gateway'", err)
	}
}

func TestUpLocal_NoProviders_RegistersProviders(t *testing.T) {
	dir := setupTestProfile(t)
	os.MkdirAll(filepath.Join(dir, "sandbox", "profiles"), 0o755)
	gw := &mockGW{
		providerList: nil,
		providers:    map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
}

func TestUpLocal_MissingProviders(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	if gw.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", gw.createCalls)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 1 || opts.Providers[0] != "github" {
		t.Errorf("Providers = %v, want [github] only", opts.Providers)
	}
}

func TestUpLocal_AllProvidersMissing(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 0 {
		t.Errorf("Providers = %v, want empty", opts.Providers)
	}
}

func TestUpLocal_ProfileNotFound(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{providerList: []string{"github"}}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "nonexistent",
		noTTY:       true,

	})
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestUpLocal_SandboxCreateRetry(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
		createErr:    fmt.Errorf("supervisor race"),
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

		retrySleep:  0,
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	if gw.createCalls != 2 {
		t.Errorf("createCalls = %d, want 2 (first fails, second succeeds)", gw.createCalls)
	}
	if len(gw.deletedNames) != 1 {
		t.Errorf("deletedNames = %v, want 1 cleanup delete", gw.deletedNames)
	}
}

func TestActiveGatewayInfo_NoGateway(t *testing.T) {
	gw := &mockGW{}

	_, err := activeGatewayInfo(gw)
	if err == nil {
		t.Fatal("expected error when no gateway is active")
	}
	if !strings.Contains(err.Error(), "no active gateway") {
		t.Errorf("error = %q, want 'no active gateway'", err)
	}
}

func TestActiveGatewayInfo_LocalGateway(t *testing.T) {
	gw := &mockGW{
		gatewayListResult: []gateway.GatewayInfo{
			{Name: "local", Endpoint: "127.0.0.1:17670", Active: true},
		},
	}

	info, err := activeGatewayInfo(gw)
	if err != nil {
		t.Fatalf("activeGatewayInfo: %v", err)
	}
	if info.Name != "local" {
		t.Errorf("Name = %q, want local", info.Name)
	}
	if !strings.Contains(info.Endpoint, "127.0.0.1") {
		t.Errorf("Endpoint = %q, want 127.0.0.1", info.Endpoint)
	}
}

func TestProfileHasCustomProviders(t *testing.T) {
	allProviders := []preflight.Provider{
		{Name: "github", Type: "openshell"},
		{Name: "vertex-local", Type: "openshell"},
		{Name: "gws", Type: "custom"},
	}

	if profileHasCustomProviders([]string{"github"}, allProviders) {
		t.Error("github is openshell, not custom")
	}
	if !profileHasCustomProviders([]string{"github", "gws"}, allProviders) {
		t.Error("gws is custom, should return true")
	}
	if profileHasCustomProviders([]string{}, allProviders) {
		t.Error("empty profile should not have custom providers")
	}
}

func TestUpLocal_SandboxCreateOpts(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github", "vertex-local"},
		providers:    map[string]bool{"github": true, "vertex-local": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		sandboxName: "custom-name",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if opts.Name != "custom-name" {
		t.Errorf("Name = %q, want custom-name", opts.Name)
	}
	if opts.From != "quay.io/test:latest" {
		t.Errorf("From = %q", opts.From)
	}
	if opts.TTY {
		t.Error("TTY = true, want false (noTTY)")
	}
	if !opts.Keep {
		t.Error("Keep = false, want true (default)")
	}
}
