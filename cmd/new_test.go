package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLocal_NoGateway(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{inferenceErr: fmt.Errorf("connection refused")}

	err := newLocal(newLocalOpts{
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

func TestNewLocal_NoProviders_RegistersProviders(t *testing.T) {
	dir := setupTestProfile(t)
	os.MkdirAll(filepath.Join(dir, "sandbox", "profiles"), 0o755)
	gw := &mockGW{
		providerList: nil,
		providers:    map[string]bool{},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
}

func TestNewLocal_MissingProviders(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	if gw.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", gw.createCalls)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 1 || opts.Providers[0] != "github" {
		t.Errorf("Providers = %v, want [github] only", opts.Providers)
	}
}

func TestNewLocal_AllProvidersMissing(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 0 {
		t.Errorf("Providers = %v, want empty", opts.Providers)
	}
}

func TestNewLocal_ProfileNotFound(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{providerList: []string{"github"}}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "nonexistent",
		noTTY:       true,

	})
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestNewLocal_SandboxCreateRetry(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
		createErr:    fmt.Errorf("supervisor race"),
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		noTTY:       true,

		retrySleep:  0,
	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	if gw.createCalls != 2 {
		t.Errorf("createCalls = %d, want 2 (first fails, second succeeds)", gw.createCalls)
	}
	if len(gw.deletedNames) != 1 {
		t.Errorf("deletedNames = %v, want 1 cleanup delete", gw.deletedNames)
	}
}

func TestNewLocal_SandboxCreateOpts(t *testing.T) {
	dir := setupTestProfile(t)
	gw := &mockGW{
		providerList: []string{"github", "vertex-local"},
		providers:    map[string]bool{"github": true, "vertex-local": true},
	}

	err := newLocal(newLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		profileName: "default",
		sandboxName: "custom-name",
		noTTY:       true,

	})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if opts.Name != "custom-name" {
		t.Errorf("Name = %q, want custom-name", opts.Name)
	}
	if opts.Image != "quay.io/test:latest" {
		t.Errorf("Image = %q", opts.Image)
	}
	if opts.TTY {
		t.Error("TTY = true, want false (noTTY)")
	}
	if !opts.Keep {
		t.Error("Keep = false, want true (default)")
	}
}
