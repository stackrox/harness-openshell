package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/agent"
	"gopkg.in/yaml.v3"
)

var testDefaultConfig = []byte(`name: test-agent
entrypoint: claude
tty: true
providers:
  - profile: google-vertex-ai
env:
  ANTHROPIC_BASE_URL: https://inference.local
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

	var cfg agent.AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("Name = %q, want test-agent", cfg.Name)
	}
	if cfg.Entrypoint != "claude" {
		t.Errorf("Entrypoint = %q, want claude", cfg.Entrypoint)
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

	var cfg agent.AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if cfg.Entrypoint != "claude" {
		t.Errorf("Entrypoint = %q, want claude (default)", cfg.Entrypoint)
	}
}

func TestInitRun_InteractiveOpenCode(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "opencode\n1\nlocal\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Entrypoint != "opencode" {
		t.Errorf("Entrypoint = %q, want opencode", cfg.Entrypoint)
	}
}

func TestInitRun_InteractiveProvidersSingle(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\nlocal\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if len(cfg.Providers) != 1 {
		t.Fatalf("Providers count = %d, want 1", len(cfg.Providers))
	}
}

func TestInitRun_InteractiveProvidersMultiple(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1,3\nlocal\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers count = %d, want 2", len(cfg.Providers))
	}
}

func TestInitRun_InteractiveProvidersNone(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\nnone\nlocal\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if len(cfg.Providers) != 0 {
		t.Errorf("Providers count = %d, want 0 for 'none'", len(cfg.Providers))
	}
}

func TestInitRun_InteractiveGatewayKind(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\nkind\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Gateway != "kind" {
		t.Errorf("Gateway = %q, want kind", cfg.Gateway)
	}
}

func TestInitRun_InteractiveGatewayOCP(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\nocp\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err != nil {
		t.Fatalf("initRun: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var cfg agent.AgentConfig
	yaml.Unmarshal(data, &cfg)
	if cfg.Gateway != "ocp" {
		t.Errorf("Gateway = %q, want ocp", cfg.Gateway)
	}
}

func TestInitRun_InvalidGateway(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\nbadtarget\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err == nil {
		t.Fatal("expected error for invalid gateway target")
	}
}

func TestInitRun_RemoteIsInvalidGateway(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "harness.yaml")
	var buf bytes.Buffer

	input := "claude\n1\nremote\n"
	err := initRun(strings.NewReader(input), &buf, outPath, false, false, testDefaultConfig)
	if err == nil {
		t.Fatal("expected error: 'remote' is not a valid gateway, use 'ocp'")
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

	found := make(map[string]bool)
	for _, p := range providers {
		found[p.ID] = true
	}
	for _, id := range []string{"google-vertex-ai", "github", "atlassian"} {
		if !found[id] {
			t.Errorf("missing provider %q in parsed output", id)
		}
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
