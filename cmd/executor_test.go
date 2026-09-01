package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/gateway"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

// healthyFactory returns an SDK Factory whose client reports a healthy gateway,
// so upLocal's reachability preflight passes. Reachability is the Factory's job
// now (client.Health), not the mockGW's.
func healthyFactory() openshell.Factory {
	return keepOpenFactory(testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true})))
}

// A gateway that answers but is unhealthy must abort apply at the reachability
// preflight — the harness runs only against a healthy, already-provisioned
// gateway.
func TestUpLocal_NoGateway(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{}

	unhealthy := keepOpenFactory(testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: false})))

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  unhealthy,
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
		providers: map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  healthyFactory(),
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
}

func TestUpLocal_MissingProviders(t *testing.T) {
	dir := setupTestAgent(t)
	gw := &mockGW{
		providers: map[string]bool{"github": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  healthyFactory(),
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
		providers: map[string]bool{},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		newClient:  healthyFactory(),
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
	gw := &mockGW{}

	// Agent config is parsed before the reachability preflight, so a missing
	// config fails here without needing a client factory.
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
		providers: map[string]bool{"github": true},
		createErr: fmt.Errorf("supervisor race"),
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		retrySleep: 0,
		newClient:  healthyFactory(),
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
		providers: map[string]bool{"github": true, "google-vertex-ai": true},
	}

	err := upLocal(upLocalOpts{
		harnessDir:  dir,
		gw:          gw,
		agentPath:   filepath.Join(dir, "agents", "default.yaml"),
		sandboxName: "custom-name",
		noTTY:       true,
		newClient:   healthyFactory(),
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

// TestUpLocal_InlineContentPayloadPathSurvivesRename is a regression test for
// issue #84: inline content payloads were written into payloadDir, which
// createSandbox renames into a staging directory. The rename invalidated the
// stored upload paths, so every SandboxCreate failed with "local path does not
// exist". The fix stages inline content in a separate temp dir that survives
// the rename — so every upload Src must still resolve at create time.
func TestUpLocal_InlineContentPayloadPathSurvivesRename(t *testing.T) {
	dir := setupTestAgent(t)

	var uploadsChecked bool
	gw := &mockGW{
		providers: map[string]bool{"github": true, "google-vertex-ai": true, "atlassian": true},
		onSandboxCreate: func(opts gateway.SandboxCreateOpts) error {
			for _, u := range opts.Uploads {
				if _, err := os.Stat(u.Src); err != nil {
					return fmt.Errorf("upload Src %q not accessible at create time: %w", u.Src, err)
				}
			}
			uploadsChecked = true
			return nil
		},
	}

	harness := &agent.Harness{
		Payloads: []agent.PayloadEntry{
			{SandboxPath: "/sandbox/opencode.json", Content: `{"model": "test"}`},
		},
	}

	err := upLocal(upLocalOpts{
		harnessDir: dir,
		gw:         gw,
		agentPath:  filepath.Join(dir, "agents", "default.yaml"),
		noTTY:      true,
		harness:    harness,
		newClient:  healthyFactory(),
	})
	if err != nil {
		t.Fatalf("upLocal: %v", err)
	}
	if !uploadsChecked {
		t.Fatal("SandboxCreate was not called; upload paths were never validated")
	}
	if gw.createCalls != 1 {
		t.Fatalf("expected exactly 1 SandboxCreate call, got %d", gw.createCalls)
	}
}
