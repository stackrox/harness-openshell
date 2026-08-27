package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
)

// TestMigrateCmd_Basic validates the end-to-end migrate command with basic legacy config.
func TestMigrateCmd_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, "legacy.yaml")
	legacyData := `name: basic-test
gateway: rc-dev
repo: https://github.com/example/repo
repo_ref: main
entrypoint: claude`
	if err := os.WriteFile(legacyPath, []byte(legacyData), 0o644); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"-f", legacyPath})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Validate output is valid v1alpha1 YAML
	outBytes := stdout.Bytes()
	if len(outBytes) == 0 {
		t.Fatalf("stdout is empty")
	}
	migrated, err := config.Parse(outBytes)
	if err != nil {
		t.Fatalf("parsing output: %v (output was: %q)", err, string(outBytes))
	}

	if migrated.Metadata.Name != "basic-test" {
		t.Errorf("name: got %q, want basic-test", migrated.Metadata.Name)
	}
	// The legacy gateway: field named a deploy profile, a concept the harness no
	// longer owns; migration drops it rather than mismapping it to a registered
	// gateway name, so spec.target.gateway is left empty for the user to set.
	if migrated.Spec.Target.Gateway != "" {
		t.Errorf("target.gateway: got %q, want empty (legacy gateway not carried)", migrated.Spec.Target.Gateway)
	}
}

// TestMigrateCmd_WithOutput validates writing to an output file.
func TestMigrateCmd_WithOutput(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, "legacy.yaml")
	outputPath := filepath.Join(tmpDir, "output.yaml")

	legacyData := `
name: output-test
gateway: rc-dev
repo: https://github.com/example/repo
repo_ref: main
entrypoint: claude
`
	if err := os.WriteFile(legacyPath, []byte(legacyData), 0o644); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"-f", legacyPath, "-o", outputPath})

	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Verify output file was created and is valid v1alpha1
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	migrated, err := config.Parse(outputData)
	if err != nil {
		t.Fatalf("parsing output: %v", err)
	}

	if migrated.Metadata.Name != "output-test" {
		t.Errorf("name: got %q, want output-test", migrated.Metadata.Name)
	}
}

// TestMigrateCmd_MissingFile validates error when -f is not provided.
func TestMigrateCmd_MissingFile(t *testing.T) {
	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{})

	stderr := new(bytes.Buffer)
	cmd.SetErr(stderr)

	err := cmd.Execute()
	if err == nil {
		t.Errorf("expected error for missing -f flag, got none")
	}
}

// TestMigrateCmd_WithWarnings validates that warnings are printed to stderr.
func TestMigrateCmd_WithWarnings(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, "legacy.yaml")

	legacyData := `name: warn-test
gateway: helm
repo: https://github.com/example/repo
entrypoint: claude
providers:
  - profile: github`
	if err := os.WriteFile(legacyPath, []byte(legacyData), 0o644); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"-f", legacyPath})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Validate output parses as v1alpha1
	outBytes := stdout.Bytes()
	if len(outBytes) == 0 {
		t.Fatalf("stdout is empty")
	}
	migrated, err := config.Parse(outBytes)
	if err != nil {
		t.Fatalf("parsing output: %v (output was: %q)", err, string(outBytes))
	}
	if migrated.Metadata.Name != "warn-test" {
		t.Errorf("name: got %q, want warn-test", migrated.Metadata.Name)
	}

	// Check that warnings were printed to stderr
	stderrOutput := stderr.String()
	if stderrOutput == "" {
		t.Errorf("expected warnings on stderr, got empty")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("warning")) {
		t.Errorf("expected 'warning' in stderr output, got: %s", stderrOutput)
	}
}

// TestMigrateCmd_FileNotFound validates error handling for nonexistent input file.
func TestMigrateCmd_FileNotFound(t *testing.T) {
	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"-f", "/nonexistent/path/legacy.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Errorf("expected error for nonexistent file, got none")
	}
}

// TestMigrateCmd_WithProviders validates that providers are converted and produce warnings.
func TestMigrateCmd_WithProviders(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, "legacy.yaml")

	legacyData := `name: provider-test
gateway: rc-dev
repo: https://github.com/example/repo
repo_ref: main
entrypoint: claude
providers:
  - profile: github-fact
  - profile: gcp-vertex`
	if err := os.WriteFile(legacyPath, []byte(legacyData), 0o644); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"-f", legacyPath})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	// Validate providers were migrated
	outBytes := stdout.Bytes()
	if len(outBytes) == 0 {
		t.Fatalf("stdout is empty")
	}
	migrated, err := config.Parse(outBytes)
	if err != nil {
		t.Fatalf("parsing output: %v (output was: %q)", err, string(outBytes))
	}

	if len(migrated.Spec.Providers) != 2 {
		t.Errorf("providers count: got %d, want 2", len(migrated.Spec.Providers))
	}

	if migrated.Spec.Providers[0].Name != "github-fact" {
		t.Errorf("providers[0].name: got %q, want github-fact", migrated.Spec.Providers[0].Name)
	}

	// Check for provider conversion warnings in stderr
	stderrOutput := stderr.String()
	providerWarnings := bytes.Count(stderr.Bytes(), []byte("providers[].profile"))
	if providerWarnings < 2 {
		t.Errorf("expected 2+ provider warnings in stderr, got %d. stderr: %s", providerWarnings, stderrOutput)
	}
}
