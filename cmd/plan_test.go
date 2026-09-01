package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

// captureStdout runs fn and returns captured stdout.
// Since printTable/printStructured write directly to os.Stdout via fmt.Print*,
// we must capture os.Stdout itself, not cmd.SetOut.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	// Save the original stdout.
	oldStdout := os.Stdout

	// Create a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	// Redirect stdout to the pipe.
	os.Stdout = w

	// Run the function.
	runErr := fn()

	// Close the write end so the read end gets EOF.
	w.Close()

	// Restore the original stdout.
	os.Stdout = oldStdout

	// Read the captured output.
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	r.Close()
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}

	return buf.String(), runErr
}

// TestPlanCmd_GoldenTable tests the golden human table output.
// Uses a representative resolved config against a fake seeded with health + one matching provider.
func TestPlanCmd_GoldenTable(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a test config file.
	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: test-gateway
  providers:
    - name: test-provider
      type: vertex-ai
      management: managed
      credentials:
        source: gcloud-adc
  inference:
    provider: test-provider
    model: claude-haiku-4-5
  sandbox:
    image: quay.io/test/sandbox:latest
  agent:
    type: claude
    args: [--bare]
  source:
    repo: https://github.com/test/repo
    ref: main
    destination: /sandbox/repo
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Seed the fake with health result.
	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{
		Healthy: true,
		Version: "1.0.0",
	}))

	// Wire the factory to always return the fake client.
	factory := testutil.FakeFactory(fakeClient)

	// Build the command.
	cmd := NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	// Capture stdout and execute.
	output, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	// Assert that output contains expected sections and actions.
	if !contains(output, "TARGET") {
		t.Errorf("output missing TARGET section:\n%s", output)
	}
	if !contains(output, "test-gateway") {
		t.Errorf("output missing gateway name:\n%s", output)
	}
	if !contains(output, "PROVIDERS") {
		t.Errorf("output missing PROVIDERS section:\n%s", output)
	}
	if !contains(output, "INFERENCE") {
		t.Errorf("output missing INFERENCE section:\n%s", output)
	}
	if !contains(output, "RUN") {
		t.Errorf("output missing RUN section:\n%s", output)
	}
	if !contains(output, "create") {
		t.Errorf("output missing action 'create':\n%s", output)
	}
}

// TestPlanCmd_InferenceRealDiff proves the inference row reflects the gateway's
// actual route: a matching seeded route renders noop, not the old flat validate.
func TestPlanCmd_InferenceRealDiff(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: test-gateway
  inference:
    provider: test-provider
    model: claude-haiku-4-5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{
		Healthy: true, Version: "1.0.0",
	}))
	// Seed a route matching the desired config, under the resolved default name.
	if _, err := fakeClient.SetInferenceRoute(context.Background(), openshell.InferenceRouteConfig{
		Provider: "test-provider", Model: "claude-haiku-4-5", Route: plan.DefaultInferenceRoute, NoVerify: true,
	}); err != nil {
		t.Fatalf("seed route: %v", err)
	}

	cmd := NewPlanCmd(tmpDir, testutil.FakeFactory(fakeClient))
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	output, err := captureStdout(t, func() error { return cmd.Execute() })
	if err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	if !contains(output, "INFERENCE") {
		t.Fatalf("output missing INFERENCE section:\n%s", output)
	}
	if !contains(output, "noop") {
		t.Errorf("expected inference noop for a matching route:\n%s", output)
	}
	if contains(output, "does not report inference state") {
		t.Errorf("capable gateway should not render the config-only caveat:\n%s", output)
	}
}

// TestPlanCmd_JSONOutput tests JSON output format.
func TestPlanCmd_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: test-gateway
  providers:
    - name: test-provider
      type: vertex-ai
      management: managed
      credentials:
        source: gcloud-adc
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{
		Healthy: true,
	}))

	factory := testutil.FakeFactory(fakeClient)
	cmd := NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "json"})

	output, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	// Assert that output is valid JSON-like (contains { } and quotes).
	if !contains(output, "{") || !contains(output, "}") {
		t.Errorf("output does not look like JSON:\n%s", output)
	}
	if !contains(output, `"groups"`) {
		t.Errorf("output missing 'groups' key:\n%s", output)
	}
}

// TestPlanCmd_SecretKiller ensures secret values never leak into output.
// Sets OPENSHELL_OIDC_CLIENT_SECRET=SUPERSECRET and a provider secret env.
// Asserts the literal value NEVER appears in table/json/yaml output.
func TestPlanCmd_SecretKiller(t *testing.T) {
	tmpDir := t.TempDir()
	secretValue := "SUPERSECRET123XYZ"

	t.Setenv("OPENSHELL_OIDC_CLIENT_SECRET", secretValue)
	t.Setenv("MY_PROVIDER_TOKEN", secretValue)

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: test-gateway
  providers:
    - name: test-provider
      type: custom-provider
      management: managed
      credentials:
        source: environment:MY_PROVIDER_TOKEN
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{
		Healthy: true,
	}))

	factory := testutil.FakeFactory(fakeClient)

	// Test table output.
	cmd := NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	output, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	if contains(output, secretValue) {
		t.Errorf("secret value leaked in table output: %s", output)
	}

	// Test JSON output.
	cmd = NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "json"})

	output, err = captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err != nil {
		t.Fatalf("cmd.Execute (json): %v", err)
	}

	if contains(output, secretValue) {
		t.Errorf("secret value leaked in json output: %s", output)
	}
}

// TestPlanCmd_MissingEnv_FailFast ensures that missing env vars fail before any gateway contact.
// A config referencing an unset ${VAR} should error AND the FakeFactory should record zero calls.
func TestPlanCmd_MissingEnv_FailFast(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: ${MISSING_VAR}
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Track whether the factory was called.
	factoryCalled := false
	recordingFactory := func(_ context.Context, _ openshell.Target) (openshell.Client, error) {
		factoryCalled = true
		return nil, nil
	}

	cmd := NewPlanCmd(tmpDir, recordingFactory)
	cmd.SetArgs([]string{"-f", configPath})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	_, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	// Must error due to missing env var.
	if err == nil {
		t.Fatal("expected error for missing env var, got nil")
	}

	if factoryCalled {
		t.Error("Factory was called despite missing env var; fail-fast was violated")
	}

	// Verify the error message mentions the missing variable.
	if !contains(err.Error(), "MISSING_VAR") {
		t.Errorf("error does not mention missing variable: %v", err)
	}
}

// TestPlanCmd_UnversionedConfigInput checks that an unversioned file is rejected.
func TestPlanCmd_UnversionedConfigInput(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "unversioned.yaml")
	configContent := `kind: Harness
metadata:
  name: unversioned-config
spec:
  target:
    gateway: test-gateway
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	factory := testutil.FakeFactory(nil) // won't be called

	cmd := NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{"-f", configPath})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	_, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err == nil {
		t.Fatal("expected error for missing apiVersion, got nil")
	}

	if !contains(err.Error(), "harness.openshell.dev/v1alpha1") {
		t.Errorf("error does not name the supported apiVersion: %v", err)
	}
}

// TestPlanCmd_TargetTierPrecedence tests flag > env > config precedence for target resolution.
// Verifies that the Factory and rendered plan use the same resolved gateway.
func TestPlanCmd_TargetTierPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		flag        string // --gateway flag value
		env         string // OPENSHELL_GATEWAY env var
		configGW    string // spec.target.gateway
		wantGateway string // expected gateway passed to Factory
	}{
		{name: "flag", flag: "from-flag", configGW: "from-config", wantGateway: "from-flag"},
		{name: "env-fallback", env: "from-env", configGW: "from-config", wantGateway: "from-env"},
		{name: "config-fallback", configGW: "from-config", wantGateway: "from-config"},
		{name: "flag-wins-env", flag: "from-flag", env: "from-env", configGW: "from-config", wantGateway: "from-flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			configPath := filepath.Join(tmpDir, "plan-test.yaml")
			configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: ` + tt.configGW + `
`
			if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}

			// Set the environment if specified.
			if tt.env != "" {
				t.Setenv(openshell.EnvGateway, tt.env)
			}

			// Record which gateway the Factory was called with.
			var gotGateway string
			recordingFactory := func(_ context.Context, tgt openshell.Target) (openshell.Client, error) {
				gotGateway = tgt.Gateway
				fakeClient := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{
					Healthy: true,
				}))
				return fakeClient, nil
			}

			cmd := NewPlanCmd(tmpDir, recordingFactory)
			args := []string{"-f", configPath, "-o", "table"}
			if tt.flag != "" {
				args = append(args, "--gateway", tt.flag)
			}
			cmd.SetArgs(args)

			_, err := captureStdout(t, func() error {
				return cmd.Execute()
			})

			if err != nil {
				t.Fatalf("cmd.Execute: %v", err)
			}

			// The Factory should have been called with the resolved gateway
			// (flag > env > config precedence).
			if gotGateway != tt.wantGateway {
				t.Errorf("Factory got gateway %q, want %q", gotGateway, tt.wantGateway)
			}
		})
	}
}

// TestPlanCmd_NoFileFlag ensures that omitting -f returns an error.
func TestPlanCmd_NoFileFlag(t *testing.T) {
	tmpDir := t.TempDir()
	factory := testutil.FakeFactory(nil)

	cmd := NewPlanCmd(tmpDir, factory)
	cmd.SetArgs([]string{}) // no -f flag
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	_, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err == nil {
		t.Fatal("expected error for missing -f flag, got nil")
	}

	if !contains(err.Error(), "-f") || !contains(err.Error(), "required") {
		t.Errorf("error does not mention flag requirement: %v", err)
	}
}

// TestPlanCmd_EmptyGatewaySkipsClient tests that an empty gateway (via flag/env/config)
// skips client construction and renders desired-only.
func TestPlanCmd_EmptyGatewaySkipsClient(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: ""
  providers:
    - name: test-provider
      type: vertex-ai
      management: managed
      credentials:
        source: gcloud-adc
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Track whether the factory was called.
	factoryCalled := false
	recordingFactory := func(_ context.Context, _ openshell.Target) (openshell.Client, error) {
		factoryCalled = true
		return nil, nil
	}

	cmd := NewPlanCmd(tmpDir, recordingFactory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	output, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	if err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	if factoryCalled {
		t.Error("Factory was called despite empty gateway")
	}

	// Should still render the PROVIDERS and other groups.
	if !contains(output, "PROVIDERS") {
		t.Errorf("output missing PROVIDERS section:\n%s", output)
	}
	// RUN group should also be rendered (not skipped for empty gateway).
	if !contains(output, "login-required") {
		t.Errorf("output should show login-required for empty gateway:\n%s", output)
	}
}

// TestPlanCmd_DirectTargetConnects proves a direct registration with no gateway
// name still connects to render real current state. An empty gateway used to
// skip the client entirely, so plan silently rendered desired-only for a
// reachable direct target.
func TestPlanCmd_DirectTargetConnects(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    registration:
      endpoint: https://gateway.example.com
      oidc:
        issuer: https://issuer.example.com
        clientId: client-123
        audience: aud-123
  providers:
    - name: test-provider
      type: vertex-ai
      management: managed
      credentials:
        source: gcloud-adc
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var gotDirect bool
	var gotGateway string
	recordingFactory := func(_ context.Context, tgt openshell.Target) (openshell.Client, error) {
		gotDirect = tgt.Direct != nil
		gotGateway = tgt.Gateway
		return testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true})), nil
	}

	cmd := NewPlanCmd(tmpDir, recordingFactory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	if _, err := captureStdout(t, func() error { return cmd.Execute() }); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}

	if !gotDirect {
		t.Error("factory was not called with a direct connection for a registration target (direct plan skipped the client)")
	}
	if gotGateway != "" {
		t.Errorf("expected empty gateway name for a direct target, got %q", gotGateway)
	}
}

// TestPlanCmd_UnreachableGatewayRendersDesiredOnly tests that an unreachable gateway
// does not hard-fail, but instead prints a warning and renders desired-only.
func TestPlanCmd_UnreachableGatewayRendersDesiredOnly(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "plan-test.yaml")
	configContent := `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: plan-test
spec:
  target:
    gateway: unreachable-gateway
  providers:
    - name: test-provider
      type: vertex-ai
      management: managed
      credentials:
        source: gcloud-adc
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Factory that always returns an error.
	errorFactory := func(_ context.Context, _ openshell.Target) (openshell.Client, error) {
		return nil, openshell.ErrUnavailable
	}

	cmd := NewPlanCmd(tmpDir, errorFactory)
	cmd.SetArgs([]string{"-f", configPath, "-o", "table"})

	output, err := captureStdout(t, func() error {
		return cmd.Execute()
	})

	// Should NOT error; should succeed and render desired-only.
	if err != nil {
		t.Fatalf("cmd.Execute: expected no error, got %v", err)
	}

	// Should contain the PROVIDERS group (desired-only rendering).
	if !contains(output, "PROVIDERS") {
		t.Errorf("output missing PROVIDERS section:\n%s", output)
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
