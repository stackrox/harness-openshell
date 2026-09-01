package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

var testDefaultConfig = []byte(`apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: test-agent
spec:
  target: {}
  providers:
    - name: google-vertex-ai
      management: referenced
  sandbox:
    providers: [google-vertex-ai]
    env:
      ANTHROPIC_BASE_URL: https://inference.local
    tty: true
  agent:
    type: claude
`)

func TestInitRun_NonInteractive(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	err := initRun(strings.NewReader(""), &buf, outPath, false, true, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("generated config does not parse: %v", err)
	}
	if _, err := config.Resolve(cfg, os.Getenv); err != nil {
		t.Fatalf("generated config does not validate: %v", err)
	}
	if cfg.Metadata.Name != "test-agent" {
		t.Errorf("metadata.name = %q, want test-agent", cfg.Metadata.Name)
	}
	if cfg.Spec.Agent.Type != "claude" {
		t.Errorf("spec.agent.type = %q, want claude", cfg.Spec.Agent.Type)
	}
}

func TestInitRun_OverwriteGuard(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	os.WriteFile(outPath, []byte("existing"), 0o644)
	var buf bytes.Buffer

	err := initRun(strings.NewReader(""), &buf, outPath, false, true, testDefaultConfig)
	if err == nil {
		t.Fatal("expected error for existing file without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err)
	}
}

func TestInitRun_OverwriteWithForce(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	os.WriteFile(outPath, []byte("existing"), 0o644)
	var buf bytes.Buffer

	err := initRun(strings.NewReader(""), &buf, outPath, true, true, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun with --force: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) == "existing" {
		t.Error("file was not overwritten")
	}
}

func TestInitRun_InteractiveDefaults(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	// Empty input = accept defaults for each prompt
	input := "\n\n\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("generated config does not parse: %v", err)
	}
	if cfg.Spec.Agent.Type != "claude" {
		t.Errorf("spec.agent.type = %q, want claude (default)", cfg.Spec.Agent.Type)
	}
}

func TestInitRun_InteractiveOpenCode(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "opencode\n1\n\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	cfg := readGeneratedConfig(t, outPath)
	if cfg.Spec.Agent.Type != "opencode" {
		t.Errorf("spec.agent.type = %q, want opencode", cfg.Spec.Agent.Type)
	}
}

func TestInitRun_InteractiveProvidersSingle(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\n\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	cfg := readGeneratedConfig(t, outPath)
	if len(cfg.Spec.Providers) != 1 || len(cfg.Spec.Sandbox.Providers) != 1 {
		t.Fatalf("provider counts = desired %d, sandbox %d; want 1 each", len(cfg.Spec.Providers), len(cfg.Spec.Sandbox.Providers))
	}
	if cfg.Spec.Providers[0].Name != "github" || cfg.Spec.Sandbox.Providers[0] != "github" {
		t.Fatalf("provider values = desired %q, sandbox %q; want github", cfg.Spec.Providers[0].Name, cfg.Spec.Sandbox.Providers[0])
	}
}

func TestInitRun_InteractiveProvidersMultiple(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1,3\n\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	cfg := readGeneratedConfig(t, outPath)
	if len(cfg.Spec.Providers) != 2 || len(cfg.Spec.Sandbox.Providers) != 2 {
		t.Fatalf("provider counts = desired %d, sandbox %d; want 2 each", len(cfg.Spec.Providers), len(cfg.Spec.Sandbox.Providers))
	}
	for i, want := range []string{"github", "atlassian"} {
		if cfg.Spec.Providers[i].Name != want || cfg.Spec.Sandbox.Providers[i] != want {
			t.Errorf("provider %d = desired %q, sandbox %q; want %q", i, cfg.Spec.Providers[i].Name, cfg.Spec.Sandbox.Providers[i], want)
		}
	}
}

func TestInitRun_InteractiveProvidersNone(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\nnone\n\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	cfg := readGeneratedConfig(t, outPath)
	if len(cfg.Spec.Providers) != 0 || len(cfg.Spec.Sandbox.Providers) != 0 {
		t.Errorf("provider counts = desired %d, sandbox %d; want 0 for 'none'", len(cfg.Spec.Providers), len(cfg.Spec.Sandbox.Providers))
	}
}

func TestInitRun_OutputContainsNextSteps(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	err := initRun(strings.NewReader(""), &buf, outPath, false, true, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "harness doctor") {
		t.Error("output should mention 'harness doctor'")
	}
	if !strings.Contains(output, "harness doctor -f "+outPath) {
		t.Error("output should tell doctor to check the generated config")
	}
	if !strings.Contains(output, "harness apply") {
		t.Error("output should mention 'harness apply'")
	}
}

func TestParseSelection_Valid(t *testing.T) {
	indices := parseSelection("1,3,4", 4)
	if len(indices) != 3 {
		t.Fatalf("len = %d, want 3", len(indices))
	}
	if indices[0] != 0 || indices[1] != 2 || indices[2] != 3 {
		t.Errorf("indices = %v, want [0 2 3]", indices)
	}
}

func TestParseSelection_OutOfRange(t *testing.T) {
	indices := parseSelection("0,5,2", 4)
	if len(indices) != 1 || indices[0] != 1 {
		t.Errorf("indices = %v, want [1] (only valid selection)", indices)
	}
}

func TestParseSelection_Invalid(t *testing.T) {
	indices := parseSelection("abc", 4)
	if len(indices) != 0 {
		t.Errorf("indices = %v, want empty for invalid input", indices)
	}
}

func TestParseListProfiles(t *testing.T) {
	output := `Available Provider Profiles:

  INFERENCE
    google-vertex-ai  Google Vertex AI               endpoints: 4  inference

  SOURCE CONTROL
    github            GitHub                         endpoints: 3

  KNOWLEDGE
    atlassian         Atlassian (Jira + Confluence)  endpoints: 3
    google-workspace  Google Workspace               endpoints: 8
`
	providers := parseListProfiles(output)
	if len(providers) < 3 {
		t.Fatalf("expected at least 3 providers, got %d: %+v", len(providers), providers)
	}

	found := make(map[string]availableProvider)
	for _, p := range providers {
		found[p.ID] = p
	}
	for _, id := range []string{"google-vertex-ai", "github", "atlassian"} {
		if _, ok := found[id]; !ok {
			t.Errorf("missing provider %q in parsed output", id)
		}
	}
	if got := found["github"].Category; got != "source-control" {
		t.Errorf("github category = %q, want source-control", got)
	}
}

func TestInitNoCredentialLeak(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	t.Setenv("ANTHROPIC_API_KEY", "sk-secret-key-12345")

	err := initRun(strings.NewReader(""), &buf, outPath, false, true, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)
	if strings.Contains(content, "sk-secret-key-12345") {
		t.Error("credential value leaked into generated YAML")
	}
}

func TestInitRun_GoldenAndPlanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var out bytes.Buffer
	defaultConfig, err := os.ReadFile("../profiles/harness-basic.yaml")
	if err != nil {
		t.Fatalf("ReadFile default scaffold: %v", err)
	}

	if err := initRun(strings.NewReader(""), &out, outPath, false, true, defaultConfig); err != nil {
		t.Fatalf("initRun: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile generated config: %v", err)
	}
	want, err := os.ReadFile("testdata/init.golden.yaml")
	if err != nil {
		t.Fatalf("ReadFile golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("generated config differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	workflow, err := loadWorkflow(outPath, "", "", applyOverrides{})
	if err != nil {
		t.Fatalf("canonical plan/apply loader rejected init output: %v", err)
	}
	if workflow.Desired.Metadata.Name != "agent" {
		t.Errorf("loaded metadata.name = %q, want agent", workflow.Desired.Metadata.Name)
	}
}

func TestInitOutputAppliesThroughActiveGateway(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "harness.yaml")
	defaultConfig, err := os.ReadFile("../profiles/harness-basic.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := initRun(strings.NewReader(""), io.Discard, path, false, true, defaultConfig); err != nil {
		t.Fatalf("initRun: %v", err)
	}
	base, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("default", &types.Provider{Name: "google-vertex-ai"})
	client := &recordingSDK{Client: base}
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		if target != (openshell.Target{}) {
			t.Fatalf("target = %+v, want active gateway", target)
		}
		return client, nil
	}
	command := NewApplyCmd(factory)
	command.SetArgs([]string{"-f", path})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("generated workflow apply: %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("create calls = %d", client.createCalls)
	}
}

func readGeneratedConfig(t *testing.T, path string) *config.Harness {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("generated config does not parse: %v", err)
	}
	return cfg
}
