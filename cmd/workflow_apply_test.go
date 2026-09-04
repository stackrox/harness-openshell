package cmd

import (
	"context"
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

	workflow, err := loadWorkflow(file, "flag-gateway", "flag-workspace", applyOverrides{})
	if err != nil {
		t.Fatalf("loadWorkflow: %v", err)
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

	sdk := &recordingSDK{Client: client}
	if err := applyWorkflow(context.Background(), workflow, planned, current, sdk, applyOptions{}); err != nil {
		t.Fatalf("applyWorkflow: %v", err)
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
	workflow, err := loadWorkflow(file, "", "", applyOverrides{})
	if err != nil {
		t.Fatalf("loadWorkflow: %v", err)
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
	if err := applyWorkflow(context.Background(), workflow, planned, current, client, applyOptions{}); err != nil {
		t.Fatalf("applyWorkflow: %v", err)
	}
}

func TestApplySetupOnlySkipsSandbox(t *testing.T) {
	desired := &config.Harness{
		Metadata: config.Metadata{Name: "setup"},
		Spec: config.Spec{
			Target:  config.Target{Gateway: "acs"},
			Sandbox: config.Sandbox{Image: "reviewer"},
			Agent:   config.Agent{Type: "reviewer"},
		},
	}
	workflow := &resolvedWorkflow{
		Desired: desired,
		Target:  openshell.Target{Gateway: "acs"},
		BaseDir: t.TempDir(),
	}
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	sdk := &recordingSDK{Client: client}
	if err := applyWorkflow(context.Background(), workflow, planned, current, sdk, applyOptions{SetupOnly: true}); err != nil {
		t.Fatalf("applyWorkflow: %v", err)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("sandbox create calls = %d, want 0", sdk.createCalls)
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

	client, raw := testutil.NewFakeClient("cli-workspace", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	raw.AddProvider("cli-workspace", &types.Provider{Name: "github", Type: "github"})
	sdk := &recordingSDK{Client: client}
	var factoryTarget openshell.Target
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		factoryTarget = target
		return sdk, nil
	}

	command := NewApplyCmd(factory)
	command.SetArgs([]string{"-f", workflowPath, "--gateway", "cli-gateway", "--workspace", "cli-workspace"})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("apply command: %v", err)
	}
	if factoryTarget != (openshell.Target{Gateway: "cli-gateway", Workspace: "cli-workspace"}) {
		t.Fatalf("factory target = %+v", factoryTarget)
	}
	if sdk.createCalls != 1 || !reflect.DeepEqual(sdk.command, []string{"reviewer", "--strict"}) {
		t.Errorf("SDK create calls=%d command=%v", sdk.createCalls, sdk.command)
	}
	if string(sdk.created.Policy) != "version: 1\n" {
		t.Errorf("SDK policy = %q", sdk.created.Policy)
	}
}

func TestApplyRequiresCanonicalFile(t *testing.T) {
	command := NewApplyCmd(testutil.FakeFactory(nil))
	command.SetArgs(nil)
	command.SilenceErrors = true
	command.SilenceUsage = true
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "-f/--file is required") {
		t.Fatalf("error = %v, want required file", err)
	}
}

func TestApplyUsesActiveGatewayWhenTargetIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	writeTestFile(t, path, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: active
spec:
  sandbox:
    image: reviewer
  agent:
    type: "true"
`)
	base := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	client := &recordingSDK{Client: base}
	called := false
	factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
		called = true
		if target != (openshell.Target{}) {
			t.Fatalf("target = %+v, want active gateway", target)
		}
		return client, nil
	}
	command := NewApplyCmd(factory)
	command.SetArgs([]string{"-f", path})
	if _, err := captureStdout(t, command.Execute); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !called || client.createCalls != 1 {
		t.Fatalf("factory called=%v create calls=%d", called, client.createCalls)
	}
}

func TestApplyStructuredOutputRedactsCredentialBearingMaps(t *testing.T) {
	secret := "secret-value-that-must-not-leak"
	t.Setenv("WORKFLOW_SECRET", secret)
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	writeTestFile(t, path, `apiVersion: harness.openshell.dev/v1alpha1
kind: Harness
metadata:
  name: redacted
spec:
  providers:
    - name: existing
      management: referenced
      config:
        API_TOKEN: ${WORKFLOW_SECRET}
  sandbox:
    env:
      API_TOKEN: ${WORKFLOW_SECRET}
  agent:
    type: sh
    args: [-c, '${WORKFLOW_SECRET}']
  source:
    repo: ${WORKFLOW_SECRET}
  payloads:
    - content: ${WORKFLOW_SECRET}
      destination: /sandbox/secret
`)
	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			command := NewApplyCmd(testutil.FakeFactory(nil))
			command.SetArgs([]string{"-f", path, "-o", format})
			output, err := captureStdout(t, command.Execute)
			if err != nil {
				t.Fatalf("apply -o %s: %v", format, err)
			}
			if strings.Contains(output, secret) {
				t.Fatalf("secret leaked in %s output: %s", format, output)
			}
			if strings.Count(output, "redacted") < 5 {
				t.Errorf("output does not redact every host-derived value: %s", output)
			}
		})
	}
}

func TestRedactedWorkflowRedactsInterpolatedScalars(t *testing.T) {
	resolved := &config.Harness{
		APIVersion: "harness.openshell.dev/v1alpha1",
		Kind:       "Harness",
		Metadata:   config.Metadata{Name: "resolved-name"},
		Spec: config.Spec{
			Target: config.Target{
				Gateway:   "resolved-gateway",
				Workspace: "workspace",
				Registration: &config.Registration{
					Endpoint: "https://gateway.example",
					OIDC:     &config.OIDC{Issuer: "https://issuer.example", ClientID: "client", Audience: "audience"},
				},
			},
			Inference: config.Inference{Route: "inference.local", Provider: "vertex", Model: "model", Timeout: "60s"},
			Sandbox: config.Sandbox{
				Image:     "image",
				Providers: []string{"provider"},
				Policy:    &config.PolicyRef{File: "policy.yaml"},
			},
			Agent:     config.Agent{Type: "agent", Args: []string{"literal", "resolved-argument"}},
			Source:    config.Source{Repo: "repo", Ref: "main", Destination: "/sandbox", Submodules: "shallow"},
			Payloads:  []config.Payload{{Source: "payload", Content: "content", Destination: "/sandbox/payload"}},
			Providers: []config.Provider{{Name: "provider", Type: "vertex", Management: "referenced"}},
		},
	}
	input := &config.Harness{
		APIVersion: resolved.APIVersion,
		Kind:       resolved.Kind,
		Metadata:   config.Metadata{Name: "${NAME}"},
		Spec: config.Spec{
			Target: config.Target{
				Gateway:   "${GATEWAY}",
				Workspace: "workspace",
				Registration: &config.Registration{
					Endpoint: "${ENDPOINT}",
					OIDC:     &config.OIDC{Issuer: "${ISSUER}", ClientID: "${CLIENT_ID}", Audience: "${AUDIENCE}"},
				},
			},
			Inference: config.Inference{Route: "${ROUTE}", Provider: "vertex", Model: "${MODEL}", Timeout: "60s"},
			Sandbox: config.Sandbox{
				Image:     "${IMAGE}",
				Providers: []string{"${PROVIDER}"},
				Policy:    &config.PolicyRef{File: "${POLICY}"},
			},
			Agent:     config.Agent{Type: "${AGENT}", Args: []string{"literal", "${ARGUMENT}"}},
			Source:    config.Source{Repo: "${REPO}", Ref: "main", Destination: "${DESTINATION}", Submodules: "shallow"},
			Payloads:  []config.Payload{{Source: "${PAYLOAD_SOURCE}", Content: "${PAYLOAD_CONTENT}", Destination: "${PAYLOAD_DESTINATION}"}},
			Providers: []config.Provider{{Name: "${PROVIDER_NAME}", Type: "vertex", Management: "${MANAGEMENT}"}},
		},
	}

	got := redactedWorkflow(resolved, input)
	if got.Metadata.Name != "<redacted>" || got.Spec.Target.Gateway != "<redacted>" || got.Spec.Target.Registration.Endpoint != "<redacted>" || got.Spec.Target.Registration.OIDC.ClientID != "<redacted>" {
		t.Errorf("target fields = %+v, want interpolated values redacted", got.Spec.Target)
	}
	if got.Spec.Inference.Route != "<redacted>" || got.Spec.Inference.Model != "<redacted>" || got.Spec.Sandbox.Image != "<redacted>" || got.Spec.Sandbox.Providers[0] != "<redacted>" || got.Spec.Sandbox.Policy.File != "<redacted>" {
		t.Errorf("inference/sandbox fields were not redacted: %+v %+v", got.Spec.Inference, got.Spec.Sandbox)
	}
	if got.Spec.Agent.Type != "<redacted>" || got.Spec.Agent.Args[0] != "literal" || got.Spec.Agent.Args[1] != "<redacted>" {
		t.Errorf("agent fields = %+v", got.Spec.Agent)
	}
	if got.Spec.Source.Repo != "<redacted>" || got.Spec.Source.Ref != "main" || got.Spec.Source.Destination != "<redacted>" {
		t.Errorf("source fields = %+v", got.Spec.Source)
	}
	if got.Spec.Payloads[0].Source != "<redacted>" || got.Spec.Payloads[0].Content != "<redacted>" || got.Spec.Payloads[0].Destination != "<redacted>" || got.Spec.Providers[0].Name != "<redacted>" || got.Spec.Providers[0].Management != "<redacted>" {
		t.Errorf("payload/provider fields were not redacted: %+v %+v", got.Spec.Payloads[0], got.Spec.Providers[0])
	}
}

func TestApplyRejectsUnversionedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	writeTestFile(t, path, "name: old-config\nentrypoint: claude\n")
	command := NewApplyCmd(testutil.FakeFactory(nil))
	command.SetArgs([]string{"-f", path})
	command.SilenceErrors = true
	command.SilenceUsage = true
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "harness.openshell.dev/v1alpha1") {
		t.Fatalf("error = %v, want supported apiVersion", err)
	}
}

type recordingSDK struct {
	openshell.Client
	createCalls int
	created     openshell.SandboxCreate
	command     []string
	deleted     bool
	interactive bool
}

func (c *recordingSDK) CreateSandbox(_ context.Context, desired openshell.SandboxCreate) (openshell.Sandbox, error) {
	c.createCalls++
	c.created = desired
	return openshell.Sandbox{Name: desired.Name, Phase: "Provisioning"}, nil
}

func (c *recordingSDK) WaitSandboxReady(_ context.Context, name string) (openshell.Sandbox, error) {
	return openshell.Sandbox{Name: name, Phase: "Ready"}, nil
}

func (*recordingSDK) UploadPath(_ context.Context, _, _, _ string) error { return nil }

func (c *recordingSDK) ExecSandbox(_ context.Context, _ string, command []string, _, _ io.Writer) (int, error) {
	c.command = append([]string(nil), command...)
	return 0, nil
}

func (c *recordingSDK) ExecInteractive(_ context.Context, _ string, command []string, _, _ uint32) (openshell.InteractiveSession, error) {
	c.interactive = true
	c.command = append([]string(nil), command...)
	return interactiveSession{}, nil
}

func (c *recordingSDK) DeleteSandbox(_ context.Context, _ string) error {
	c.deleted = true
	return nil
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
	workflow := &resolvedWorkflow{
		Desired: desired,
		Target:  openshell.Target{Gateway: "acs", Workspace: "team"},
		BaseDir: t.TempDir(),
	}
	base := testutil.NewFake("team", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	client := &recordingSDK{Client: base}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if err := applyWorkflow(context.Background(), workflow, planned, current, client, applyOptions{}); err != nil {
		t.Fatalf("applyWorkflow: %v", err)
	}
	if !client.interactive || !reflect.DeepEqual(client.command, []string{"reviewer"}) {
		t.Errorf("interactive=%v command=%v", client.interactive, client.command)
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
	workflow := &resolvedWorkflow{
		Desired: desired,
		Target:  openshell.Target{Direct: &openshell.DirectConnection{Endpoint: "https://gateway.example.com"}},
		BaseDir: t.TempDir(),
	}
	base := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	client := &recordingSDK{Client: base}
	planned, current, err := workflow.buildPlan(context.Background(), client)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if err := applyWorkflow(context.Background(), workflow, planned, current, client, applyOptions{}); err != nil {
		t.Fatalf("applyWorkflow: %v", err)
	}
	if !client.interactive {
		t.Error("interactive SDK call was not made")
	}
}

type interactiveSession struct{}

func (interactiveSession) Read([]byte) (int, error)    { return 0, io.EOF }
func (interactiveSession) Write(p []byte) (int, error) { return len(p), nil }
func (interactiveSession) Resize(uint32, uint32) error { return nil }
func (interactiveSession) ExitCode() (int, error)      { return 0, nil }
func (interactiveSession) Close() error                { return nil }

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
	newFactory := func() openshell.Factory {
		client := testutil.NewFake("team", fake.WithHealthResult(&types.HealthResult{Healthy: true, Version: "test"}))
		return testutil.FakeFactory(client)
	}

	planCommand := NewPlanCmd(newFactory())
	planCommand.SetArgs([]string{"-f", workflowPath, "-o", "json"})
	planOutput, err := captureStdout(t, planCommand.Execute)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	applyCommand := NewApplyCmd(newFactory())
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
	workflow := &resolvedWorkflow{
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
	sdk := &recordingSDK{Client: client}
	err = applyWorkflow(context.Background(), workflow, planned, current, sdk, applyOptions{})
	if err == nil || !strings.Contains(err.Error(), "referenced provider") {
		t.Fatalf("error = %v, want missing referenced provider", err)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("SDK sandbox create calls = %d, want 0", sdk.createCalls)
	}
}

func TestCanonicalApplyMissingSandboxProviderFailsBeforeSandbox(t *testing.T) {
	client := testutil.NewFake("default", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
	workflow := &resolvedWorkflow{
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
	sdk := &recordingSDK{Client: client}
	err = applyWorkflow(context.Background(), workflow, planned, current, sdk, applyOptions{})
	if err == nil || !strings.Contains(err.Error(), `verifying sandbox provider "github-read"`) {
		t.Fatalf("error = %v, want missing sandbox provider", err)
	}
	if sdk.createCalls != 0 {
		t.Fatalf("SDK sandbox create calls = %d, want 0", sdk.createCalls)
	}
}

func TestCanonicalRunRequestResolvesConfigRelativeArtifacts(t *testing.T) {
	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "instructions.md")
	policyPath := filepath.Join(dir, "policy.yaml")
	writeTestFile(t, payloadPath, "review carefully\n")
	writeTestFile(t, policyPath, "version: 1\n")

	workflow := &resolvedWorkflow{
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

	req, cleanup, err := buildRunRequest(workflow)
	if err != nil {
		t.Fatalf("buildRunRequest: %v", err)
	}
	defer cleanup()
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
