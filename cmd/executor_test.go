package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

func TestUpLocal_NoGateway(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{inferenceErr: fmt.Errorf("connection refused")}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no active gateway") {
		t.Errorf("error = %q, want 'no active gateway'", err)
	}
}

func TestUpLocal_NoProviders_RegistersProviders(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: nil,
		providers:    map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
}

func TestUpLocal_MissingProviders(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
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
	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	opts := gw.createOpts[0]
	if len(opts.Providers) != 0 {
		t.Errorf("Providers = %v, want empty", opts.Providers)
	}
}

func TestUpLocal_AgentNotFound(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{providerList: []string{"github"}}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "nonexistent.yaml"),
		noTTY:      true,
	})
	if err == nil {
		t.Fatal("expected error for missing agent config")
	}
}

func TestUpLocal_SandboxCreateRetry(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
		createErr:    fmt.Errorf("supervisor race"),
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		retrySleep: 0,
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

func TestUpLocal_SandboxCreateOpts(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: []string{"github", "google-vertex-ai"},
		providers:    map[string]bool{"github": true, "google-vertex-ai": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		agentPath:   filepath.Join(dir, "agents", "default.yaml"),
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
		t.Errorf("From = %q, want quay.io/test:latest", opts.From)
	}
	if opts.TTY {
		t.Error("TTY = true, want false (noTTY)")
	}
}

func TestUpLocal_EnsureLocal_DeploysGateway(t *testing.T) {
	lookPath = func(string) (string, error) { return "/usr/bin/podman", nil }
	t.Cleanup(func() { lookPath = exec.LookPath })

	dir := setupTestAgent(t)
	gw := &mockGW{
		providerList: []string{"github"},
		providers:    map[string]bool{"github": true},
		gatewayListResult: []gateway.GatewayInfo{
			{Name: "local", Endpoint: "127.0.0.1:17670", Active: true},
		},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		agentPath:   filepath.Join(dir, "agents", "default.yaml"),
		ensureLocal: true,
		noTTY:       true,
	})
	if err != nil {
		t.Fatalf("upLocal with ensureLocal=true: %v", err)
	}
	if gw.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", gw.createCalls)
	}
}

func TestResolveHarness_EmbeddedFallback(t *testing.T) {
	dir := t.TempDir()
	DefaultAgentConfig = []byte(`name: embedded-default
entrypoint: claude
providers:
  - profile: github
`)
	t.Cleanup(func() { DefaultAgentConfig = nil })

	h, err := resolveHarness(dir, "default", "")
	if err != nil {
		t.Fatalf("resolveHarness: %v", err)
	}
	if h.Agent.Name != "embedded-default" {
		t.Errorf("Name = %q, want embedded-default", h.Agent.Name)
	}
}

func TestResolveHarness_DiskOverridesEmbedded(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "agent-default.yaml"), []byte(`name: disk-agent
entrypoint: claude
providers:
  - profile: github
`), 0o644)

	DefaultAgentConfig = []byte(`name: embedded-default
entrypoint: claude
providers:
  - profile: github
`)
	t.Cleanup(func() { DefaultAgentConfig = nil })

	h, err := resolveHarness(dir, "default", "")
	if err != nil {
		t.Fatalf("resolveHarness: %v", err)
	}
	if h.Agent.Name != "disk-agent" {
		t.Errorf("Name = %q, want disk-agent (disk should override embedded)", h.Agent.Name)
	}
}

func TestResolveHarness_ExplicitFileNoFallback(t *testing.T) {
	dir := t.TempDir()
	DefaultAgentConfig = []byte(`name: embedded-default
entrypoint: claude
providers:
  - profile: github
`)
	t.Cleanup(func() { DefaultAgentConfig = nil })

	_, err := resolveHarness(dir, "default", "/nonexistent/agent.yaml")
	if err == nil {
		t.Fatal("expected error for explicit nonexistent --file, should not fall back to embedded")
	}
}

func TestResolveHarness_NonDefaultNameNoFallback(t *testing.T) {
	dir := t.TempDir()
	DefaultAgentConfig = []byte(`name: embedded-default
entrypoint: claude
providers:
  - profile: github
`)
	t.Cleanup(func() { DefaultAgentConfig = nil })

	_, err := resolveHarness(dir, "research", "")
	if err == nil {
		t.Fatal("expected error for --agent research when file doesn't exist, should not fall back to embedded")
	}
}
