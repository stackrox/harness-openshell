package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/testutil"
)

func TestCanonicalWorkflowPlanAndApplyShareResolvedTarget(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	t.Setenv(openshell.EnvGateway, "env-gateway")
	t.Setenv(openshell.EnvWorkspace, "env-workspace")
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow.yaml")
	writeTestFile(t, file, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: review
spec:
  target:
    gateway: config-gateway
    workspace: config-workspace
  providers:
    - name: github
      management: referenced
  sandbox:
    image: quay.io/test/reviewer:latest
    providers: [github]
    env:
      REVIEW_MODE: strict
    keep: true
  agent:
    type: reviewer
    args: [--format, sarif]
`)

	workflow, err := loadCanonicalWorkflow(file, "flag-gateway", "flag-workspace", canonicalOverrides{})
	if err != nil {
		t.Fatalf("loadCanonicalWorkflow: %v", err)
	}
	if workflow.Target != (openshell.Target{Gateway: "flag-gateway", Workspace: "flag-workspace"}) {
		t.Fatalf("target = %+v", workflow.Target)
	}

	client, raw := testutil.NewFakeClient("flag-workspace", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("flag-workspace", &types.Provider{Name: "github", Type: "github"})
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if planned.Target != workflow.Target {
		t.Fatalf("plan target = %+v, workflow target = %+v", planned.Target, workflow.Target)
	}

	gw := &mockGW{}
	if err := applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{}); err != nil {
		t.Fatalf("applyCanonical: %v", err)
	}
	if gw.createCalls != 1 {
		t.Fatalf("sandbox create calls = %d, want 1", gw.createCalls)
	}
	opts := gw.createOpts[0]
	if opts.Gateway != planned.Target.Gateway || opts.Workspace != planned.Target.Workspace {
		t.Errorf("sandbox target = %q/%q, plan target = %q/%q", opts.Gateway, opts.Workspace, planned.Target.Gateway, planned.Target.Workspace)
	}
	if !opts.NoAutoProviders {
		t.Error("v1alpha1 apply must disable automatic providers")
	}
	if !reflect.DeepEqual(opts.Command, []string{"reviewer", "--format", "sarif"}) {
		t.Errorf("command = %v", opts.Command)
	}
	if !reflect.DeepEqual(opts.Providers, []string{"github"}) {
		t.Errorf("providers = %v", opts.Providers)
	}
	if opts.Env["REVIEW_MODE"] != "strict" || !opts.Keep {
		t.Errorf("sandbox options did not preserve env/keep: %+v", opts)
	}
}

func TestCanonicalProviderOnlyWorkflowDoesNotInventSandboxRun(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow.yaml")
	writeTestFile(t, file, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: setup
spec:
  target:
    gateway: acs
  providers:
    - name: github
      management: referenced
`)
	workflow, err := loadCanonicalWorkflow(file, "", "", canonicalOverrides{})
	if err != nil {
		t.Fatalf("loadCanonicalWorkflow: %v", err)
	}
	if workflow.Desired.Spec.Sandbox.Image != "" {
		t.Fatalf("provider-only workflow image = %q, want empty", workflow.Desired.Spec.Sandbox.Image)
	}
	client, raw := testutil.NewFakeClient("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	gw := &mockGW{}
	if err := applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{}); err != nil {
		t.Fatalf("applyCanonical: %v", err)
	}
	if gw.createCalls != 0 {
		t.Fatalf("sandbox create calls = %d, want 0", gw.createCalls)
	}
}

func TestApplyCommandExecutesV1alphaWorkflow(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "workflow.yaml")
	writeTestFile(t, workflowPath, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: security-review
spec:
  target:
    gateway: config-gateway
    workspace: config-workspace
  providers:
    - name: github
      management: referenced
  sandbox:
    image: reviewer
    providers: [github]
    keep: false
  agent:
    type: reviewer
    args: [--strict]
`)

	argsLog := filepath.Join(dir, "openshell.args")
	cliPath := filepath.Join(dir, "openshell")
	writeTestFile(t, cliPath, fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "openshell v0.0.110"
  exit 0
fi
printf '%%s\n' "$@" > %s
`, argsLog))
	if err := os.Chmod(cliPath, 0o700); err != nil {
		t.Fatalf("chmod fake openshell: %v", err)
	}

	client, raw := testutil.NewFakeClient("cli-workspace", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("cli-workspace", &types.Provider{Name: "github", Type: "github"})
	var factoryTarget openshell.Target
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		factoryTarget = target
		return client, nil
	}

	command := NewApplyCmd(dir, cliPath, factory)
	command.SetArgs([]string{"-f", workflowPath, "--gateway", "cli-gateway", "--workspace", "cli-workspace"})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("apply command: %v", err)
	}
	if factoryTarget != (openshell.Target{Gateway: "cli-gateway", Workspace: "cli-workspace"}) {
		t.Fatalf("factory target = %+v", factoryTarget)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake openshell args: %v", err)
	}
	for _, want := range []string{"sandbox\ncreate\n", "--gateway\ncli-gateway\n", "--workspace\ncli-workspace\n", "--no-auto-providers\n", "reviewer\n--strict\n"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("openshell args missing %q:\n%s", want, args)
		}
	}
}

func TestPlanAndApplyDryRunRenderSameCanonicalPlan(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "workflow.yaml")
	writeTestFile(t, workflowPath, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: parity
spec:
  target:
    gateway: acs
    workspace: team
  sandbox:
    image: reviewer
  agent:
    type: reviewer
    args: [--strict]
`)
	cliPath := filepath.Join(dir, "openshell")
	writeTestFile(t, cliPath, "#!/bin/sh\necho 'openshell v0.0.110'\n")
	if err := os.Chmod(cliPath, 0o700); err != nil {
		t.Fatalf("chmod fake openshell: %v", err)
	}
	newFactory := func() openshell.Factory {
		client := testutil.NewFake("team", fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "test"}))
		return testutil.FakeFactory(client)
	}

	planCommand := NewPlanCmd(dir, newFactory())
	planCommand.SetArgs([]string{"-f", workflowPath, "-o", "json"})
	planOutput, err := captureStdout(t, planCommand.Execute)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	applyCommand := NewApplyCmd(dir, cliPath, newFactory())
	applyCommand.SetArgs([]string{"-f", workflowPath, "--dry-run", "-o", "json"})
	applyOutput, err := captureStdout(t, applyCommand.Execute)
	if err != nil {
		t.Fatalf("apply --dry-run: %v", err)
	}
	if applyOutput != planOutput {
		t.Errorf("apply --dry-run must render plan's exact action graph\nplan:\n%s\napply:\n%s", planOutput, applyOutput)
	}
}

func TestCanonicalApplyMissingReferencedProviderFailsBeforeSandbox(t *testing.T) {
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	workflow := &canonicalWorkflow{
		Desired: &config.Harness{
			Metadata: config.Metadata{Name: "review"},
			Spec: config.Spec{
				Target:    config.Target{Gateway: "acs"},
				Providers: []config.Provider{{Name: "github", Management: "referenced"}},
				Sandbox:   config.Sandbox{Image: "reviewer", Providers: []string{"github"}},
			},
		},
		Target: openshell.Target{Gateway: "acs"},
	}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	gw := &mockGW{}
	err = applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "referenced provider") {
		t.Fatalf("error = %v, want missing referenced provider", err)
	}
	if gw.createCalls != 0 {
		t.Fatalf("sandbox create calls = %d, want 0", gw.createCalls)
	}
}

func TestCanonicalApplyMissingSandboxProviderFailsBeforeSandbox(t *testing.T) {
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	workflow := &canonicalWorkflow{
		Desired: &config.Harness{
			Metadata: config.Metadata{Name: "review"},
			Spec: config.Spec{
				Target:  config.Target{Gateway: "acs"},
				Sandbox: config.Sandbox{Image: "reviewer", Providers: []string{"github-read"}},
			},
		},
		Target: openshell.Target{Gateway: "acs"},
	}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	gw := &mockGW{}
	err = applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), `verifying sandbox provider "github-read"`) {
		t.Fatalf("error = %v, want missing sandbox provider", err)
	}
	if gw.createCalls != 0 {
		t.Fatalf("sandbox create calls = %d, want 0", gw.createCalls)
	}
}

func TestCanonicalRunRequestResolvesConfigRelativeArtifacts(t *testing.T) {
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "instructions.md")
	policyPath := filepath.Join(dir, "policy.yaml")
	writeTestFile(t, payloadPath, "review carefully\n")
	writeTestFile(t, policyPath, "version: 1\n")

	workflow := &canonicalWorkflow{
		Desired: &config.Harness{
			Metadata: config.Metadata{Name: "review"},
			Spec: config.Spec{
				Target:  config.Target{Gateway: "acs", Workspace: "stackrox"},
				Sandbox: config.Sandbox{Image: "reviewer", Policy: &config.PolicyRef{File: "policy.yaml"}},
				Payloads: []config.Payload{
					{Source: "instructions.md", Destination: "/sandbox/instructions.md"},
					{Content: "inline", Destination: "/sandbox/inline.txt"},
				},
			},
		},
		Target:  openshell.Target{Gateway: "acs", Workspace: "stackrox"},
		BaseDir: dir,
	}

	req, cleanup, err := canonicalRunRequest(workflow, 0)
	if err != nil {
		t.Fatalf("canonicalRunRequest: %v", err)
	}
	defer cleanup()
	if req.PolicyPath != policyPath {
		t.Errorf("policy = %q, want %q", req.PolicyPath, policyPath)
	}
	if len(req.Uploads) != 2 || req.Uploads[0].Src != payloadPath {
		t.Fatalf("uploads = %+v", req.Uploads)
	}
	if data, err := os.ReadFile(req.Uploads[1].Src); err != nil || string(data) != "inline" {
		t.Errorf("inline payload = %q, %v", data, err)
	}
	inlinePath := req.Uploads[1].Src
	cleanup()
	if _, err := os.Stat(inlinePath); !os.IsNotExist(err) {
		t.Errorf("staged inline payload not cleaned up: %v", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
