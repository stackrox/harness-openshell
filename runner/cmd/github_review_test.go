package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/integrations/github/review"
	"github.com/stackrox/harness-openshell/runner/internal/openshell"
	"github.com/stackrox/harness-openshell/runner/internal/testutil"
)

type managedReviewClient struct {
	*recordingSDK
	reads, writes    int
	scenario         string
	cancel           context.CancelFunc
	cleanupCancelled bool
}

func (c *managedReviewClient) GetInferenceRoute(context.Context, string) (openshell.InferenceRoute, error) {
	c.reads++
	switch c.scenario {
	case "missing":
		return openshell.InferenceRoute{}, openshell.ErrNotFound
	case "unsupported":
		return openshell.InferenceRoute{}, openshell.ErrUnsupported
	case "permission":
		return openshell.InferenceRoute{}, openshell.ErrPermission
	}
	model := "model"
	if c.scenario == "mismatch" || (c.scenario == "changed-after-plan" && c.reads > 1) {
		model = "changed"
	}
	return openshell.InferenceRoute{Route: "inference.local", Provider: "vertex", Model: model}, nil
}
func (c *managedReviewClient) SetInferenceRoute(context.Context, openshell.InferenceRouteConfig) (openshell.InferenceRoute, error) {
	c.writes++
	return openshell.InferenceRoute{}, errors.New("must never administer managed inference")
}
func (c *managedReviewClient) ExecSandbox(ctx context.Context, _ string, _ []string, stdout, stderr io.Writer) (int, error) {
	_, _ = io.WriteString(stdout, "agent events")
	_, _ = io.WriteString(stderr, "agent diagnostics")
	if c.scenario == "cancel" {
		c.cancel()
		<-ctx.Done()
		return 0, ctx.Err()
	}
	if c.scenario == "exit-failure" {
		return 42, nil
	}
	return 0, nil
}
func (c *managedReviewClient) DeleteSandbox(ctx context.Context, _ string) error {
	c.deleted = true
	c.cleanupCancelled = ctx.Err() != nil
	if c.scenario == "cleanup-failure" {
		return errors.New("delete failed")
	}
	return nil
}

func TestReviewUsesManagedTargetWithoutAdministration(t *testing.T) {
	for _, scenario := range []string{"success", "missing", "mismatch", "changed-after-plan", "unsupported", "permission", "missing-provider", "cancel", "exit-failure", "cleanup-failure", "keep", "tty"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("OPENSHELL_GATEWAY", "")
			t.Setenv("OPENSHELL_WORKSPACE", "")
			t.Setenv("REVIEW_HEAD", "ambient-wrong-head")
			root := t.TempDir()
			file := filepath.Join(root, "managed.yaml")
			config := `version: 1
name: fixed-name
target:
  workspace: shared
  registration:
    endpoint: https://gateway.example
    oidc:
      issuer: https://issuer.example
      clientId: reviewer
      audience: openshell
inference:
  provider: vertex
  model: model
sandbox:
  image: test-image
  providers: [github]
  env:
    REVIEW_HEAD: ${REVIEW_HEAD}
agent:
  type: opencode
`
			if scenario == "keep" || scenario == "tty" {
				config = strings.Replace(config, "sandbox:\n", "sandbox:\n  "+scenario+": true\n", 1)
			}
			writeTestFile(t, file, config)
			client, raw := testutil.NewFakeClient("shared", fake.WithHealthResult(&types.HealthResult{Healthy: true}))
			raw.AddProvider("shared", &types.Provider{Name: "vertex", Type: "google-vertex-ai"})
			if scenario != "missing-provider" {
				raw.AddProvider("shared", &types.Provider{Name: "github", Type: "github-review"})
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sdk := &managedReviewClient{recordingSDK: &recordingSDK{Client: client}, scenario: scenario, cancel: cancel}
			connected := false
			factory := func(_ context.Context, target openshell.Target) (openshell.Client, error) {
				connected = true
				if target.Direct == nil || target.Direct.Endpoint != "https://gateway.example" || target.Workspace != "shared" {
					t.Fatalf("lost managed target: %+v", target)
				}
				return sdk, nil
			}
			var stdout, stderr bytes.Buffer
			err := reviewExecutor(factory)(ctx, review.Task{File: file, Name: "review-unique", ResultFile: filepath.Join(root, "execution.json"), Variables: map[string]string{"REVIEW_HEAD": "prepared-head"}}, &stdout, &stderr)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("execution error: %v", err)
			}
			if sdk.writes != 0 {
				t.Fatal("managed inference was changed")
			}
			wantRun := scenario == "success" || scenario == "cancel" || scenario == "exit-failure" || scenario == "cleanup-failure"
			if (sdk.createCalls == 1) != wantRun || sdk.deleted != wantRun {
				t.Fatalf("created=%d deleted=%v", sdk.createCalls, sdk.deleted)
			}
			if sdk.cleanupCancelled {
				t.Fatal("cleanup inherited cancelled context")
			}
			if wantRun && (sdk.created.Name != "review-unique" || sdk.created.Env["REVIEW_HEAD"] != "prepared-head") {
				t.Fatalf("wrong resolved request: %+v", sdk.created)
			}
			if scenario == "success" && (stdout.String() != "agent events" || stderr.String() != "agent diagnostics") {
				t.Fatalf("wrong stream routing: %q %q", stdout.String(), stderr.String())
			}
			if (scenario == "keep" || scenario == "tty") && connected {
				t.Fatal("invalid review mode reached gateway")
			}
			result := readApplyResult(t, filepath.Join(root, "execution.json"))
			if (result.Status == "succeeded") != (scenario == "success") {
				t.Fatalf("wrong result: %+v", result)
			}
		})
	}
}
