package cmd

import (
	"context"
	"fmt"
	"io"
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

	sdk := &recordingCanonicalSDK{Client: client}
	gw := &mockGW{}
	if err := applyCanonical(context.Background(), workflow, planned, current, sdk, gw, canonicalApplyOptions{}); err != nil {
		t.Fatalf("applyCanonical: %v", err)
	}
	if gw.createCalls != 0 {
		t.Fatalf("CLI sandbox create calls = %d, want 0", gw.createCalls)
	}
	if sdk.createCalls != 1 || sdk.created.Name != "review" || sdk.created.Image != "quay.io/test/reviewer:latest" {
		t.Errorf("SDK create calls=%d request=%+v", sdk.createCalls, sdk.created)
	}
	if !reflect.DeepEqual(sdk.command, []string{"reviewer", "--format", "sarif"}) {
		t.Errorf("command = %v", sdk.command)
	}
	if !reflect.DeepEqual(sdk.created.Providers, []string{"github"}) {
		t.Errorf("providers = %v", sdk.created.Providers)
	}
	if sdk.created.Env["REVIEW_MODE"] != "strict" || sdk.deleted {
		t.Errorf("sandbox options did not preserve env/keep: %+v deleted=%v", sdk.created, sdk.deleted)
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
	writeTestFile(t, filepath.Join(dir, "policy.yaml"), "version: 1\n")
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
    policy:
      file: policy.yaml
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
	sdk := &recordingCanonicalSDK{Client: client}
	var factoryTarget openshell.Target
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		factoryTarget = target
		return sdk, nil
	}

	command := NewApplyCmd(dir, cliPath, factory)
	command.SetArgs([]string{"-f", workflowPath, "--gateway", "cli-gateway", "--workspace", "cli-workspace"})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("apply command: %v", err)
	}
	if factoryTarget != (openshell.Target{Gateway: "cli-gateway", Workspace: "cli-workspace"}) {
		t.Fatalf("factory target = %+v", factoryTarget)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("openshell CLI was invoked on SDK-native path: %v", err)
	}
	if sdk.createCalls != 1 || !reflect.DeepEqual(sdk.command, []string{"reviewer", "--strict"}) {
		t.Errorf("SDK create calls=%d command=%v", sdk.createCalls, sdk.command)
	}
	if string(sdk.created.Policy) != "version: 1\n" {
		t.Errorf("SDK policy = %q", sdk.created.Policy)
	}
}

type recordingCanonicalSDK struct {
	openshell.Client
	createCalls int
	created     openshell.SandboxCreate
	command     []string
	deleted     bool
	interactive bool
}

func (c *recordingCanonicalSDK) CreateSandbox(_ context.Context, desired openshell.SandboxCreate) (openshell.Sandbox, error) {
	c.createCalls++
	c.created = desired
	return openshell.Sandbox{Name: desired.Name, Phase: "Provisioning"}, nil
}

func (c *recordingCanonicalSDK) WaitSandboxReady(_ context.Context, name string) (openshell.Sandbox, error) {
	return openshell.Sandbox{Name: name, Phase: "Ready"}, nil
}

func (*recordingCanonicalSDK) UploadPath(_ context.Context, _, _, _ string) error { return nil }

func (c *recordingCanonicalSDK) ExecSandbox(_ context.Context, _ string, command []string, _, _ io.Writer) (int, error) {
	c.command = append([]string(nil), command...)
	return 0, nil
}

func (c *recordingCanonicalSDK) ExecInteractive(_ context.Context, _ string, command []string, _, _ uint32) (openshell.InteractiveSession, error) {
	c.interactive = true
	c.command = append([]string(nil), command...)
	return canonicalInteractiveSession{}, nil
}

func (c *recordingCanonicalSDK) DeleteSandbox(_ context.Context, _ string) error {
	c.deleted = true
	return nil
}

func TestCanonicalSDKRunEligibility(t *testing.T) {
	desired := &config.Harness{Spec: config.Spec{Sandbox: config.Sandbox{Image: "quay.io/example/reviewer:v1"}}}
	workflow := &canonicalWorkflow{Desired: desired, BaseDir: t.TempDir()}
	if !canonicalSDKRunEligible(workflow) {
		t.Fatal("remote-image non-interactive workflow should use SDK")
	}

	desired.Spec.Payloads = []config.Payload{{Content: "review", Destination: "/sandbox/review.md"}}
	if !canonicalSDKRunEligible(workflow) {
		t.Fatal("payload upload should use SDK")
	}
	desired.Spec.Payloads = nil
	desired.Spec.Source.Repo = "https://example.com/repo.git"
	if !canonicalSDKRunEligible(workflow) {
		t.Fatal("source upload should use SDK")
	}
	desired.Spec.Source.Repo = ""
	desired.Spec.Sandbox.Policy = &config.PolicyRef{File: "policy.yaml"}
	if !canonicalSDKRunEligible(workflow) {
		t.Fatal("policy-bearing workflow should use SDK")
	}
	desired.Spec.Sandbox.Policy = nil
	desired.Spec.Sandbox.TTY = true
	if !canonicalSDKRunEligible(workflow) {
		t.Fatal("interactive workflow should use SDK")
	}
}

func TestCanonicalApplyUsesSDKForTTY(t *testing.T) {
	desired := &config.Harness{
		Metadata: config.Metadata{Name: "interactive"},
		Spec: config.Spec{
			Target:  config.Target{Gateway: "acs", Workspace: "team"},
			Sandbox: config.Sandbox{Image: "reviewer", TTY: true},
			Agent:   config.Agent{Type: "reviewer"},
		},
	}
	workflow := &canonicalWorkflow{
		Desired: desired,
		Target:  openshell.Target{Gateway: "acs", Workspace: "team"},
		BaseDir: t.TempDir(),
	}
	base := testutil.NewFake("team", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	client := &recordingCanonicalSDK{Client: base}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	gw := &mockGW{}
	if err := applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{}); err != nil {
		t.Fatalf("applyCanonical: %v", err)
	}
	if !client.interactive || !reflect.DeepEqual(client.command, []string{"reviewer"}) || gw.createCalls != 0 {
		t.Errorf("interactive=%v command=%v CLI create calls=%d", client.interactive, client.command, gw.createCalls)
	}
}

func TestCanonicalApplyUsesSDKTTYForDirectTarget(t *testing.T) {
	desired := &config.Harness{
		Metadata: config.Metadata{Name: "interactive"},
		Spec: config.Spec{
			Sandbox: config.Sandbox{Image: "reviewer", TTY: true},
			Agent:   config.Agent{Type: "reviewer"},
		},
	}
	workflow := &canonicalWorkflow{
		Desired: desired,
		Target:  openshell.Target{Direct: &openshell.DirectConnection{Endpoint: "https://gateway.example.com"}},
		BaseDir: t.TempDir(),
	}
	base := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	client := &recordingCanonicalSDK{Client: base}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	gw := &mockGW{}
	if err := applyCanonical(context.Background(), workflow, planned, current, client, gw, canonicalApplyOptions{}); err != nil {
		t.Fatalf("applyCanonical: %v", err)
	}
	if !client.interactive || gw.createCalls != 0 {
		t.Errorf("interactive=%v CLI create calls=%d", client.interactive, gw.createCalls)
	}
}

type canonicalInteractiveSession struct{}

func (canonicalInteractiveSession) Read([]byte) (int, error)    { return 0, io.EOF }
func (canonicalInteractiveSession) Write(p []byte) (int, error) { return len(p), nil }
func (canonicalInteractiveSession) Resize(uint32, uint32) error { return nil }
func (canonicalInteractiveSession) ExitCode() (int, error)      { return 0, nil }
func (canonicalInteractiveSession) Close() error                { return nil }

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
	if string(req.Policy) != "version: 1\n" {
		t.Errorf("policy bytes = %q", req.Policy)
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
