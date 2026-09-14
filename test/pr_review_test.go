package test

import (
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"testing"
)

func TestGitHubAppTokenIsHostOnly(t *testing.T) {
	script := string(mustRead(t, "../scripts/pr-review-local.sh"))
	for _, expected := range []string{"--name github-review --type github-review --credential GITHUB_TOKEN", "provider profile import", "tasks/github-pr-reviewer/openshell/providers/github-review.yaml", "provider profile delete"} {
		if !strings.Contains(script, expected) {
			t.Fatalf("local setup missing %q", expected)
		}
	}
	caller := string(mustRead(t, "../.github/workflows/ai-review.yml"))
	callerWorkflow := parseWorkflow(t, caller)
	reviewJob, ok := callerWorkflow.Jobs["review"]
	if !ok {
		t.Fatal("caller does not define a review job")
	}
	const usesPrefix = "stackrox/harness-openshell/.github/workflows/pr-review-reusable.yml@"
	pinnedRef := strings.TrimPrefix(reviewJob.Uses, usesPrefix)
	if !isSHA(pinnedRef) {
		t.Fatalf("caller does not pin the shared review workflow to a commit: %q", pinnedRef)
	}
	if reviewJob.With["harness-ref"] != pinnedRef {
		t.Fatalf("caller harness-ref does not match workflow pin %q", pinnedRef)
	}
	if reviewJob.With["openshell-github-app-client-id"] != "${{ vars.OPENSHELL_GITHUB_APP_CLIENT_ID }}" {
		t.Fatal("caller does not pass the GitHub App client ID variable")
	}
	for name, want := range map[string]string{
		"VERTEX_AI_SERVICE_ACCOUNT_KEY":    "${{ secrets.VERTEX_AI_SERVICE_ACCOUNT_KEY }}",
		"OPENSHELL_GITHUB_APP_PRIVATE_KEY": "${{ secrets.OPENSHELL_GITHUB_APP_PRIVATE_KEY }}",
	} {
		if reviewJob.Secrets[name] != want {
			t.Fatalf("caller secret %s is not explicitly forwarded", name)
		}
	}
	if len(reviewJob.Steps) != 0 {
		t.Fatal("caller still contains shared review steps")
	}

	sharedWorkflow := parseWorkflow(t, string(mustRead(t, "../.github/workflows/pr-review-reusable.yml")))
	sharedJob, ok := sharedWorkflow.Jobs["review"]
	if !ok {
		t.Fatal("shared review workflow does not define a review job")
	}
	sharedTrigger, ok := sharedWorkflow.On["workflow_call"]
	if !ok {
		t.Fatal("shared review workflow is not callable")
	}
	for _, name := range []string{"VERTEX_AI_SERVICE_ACCOUNT_KEY", "OPENSHELL_GITHUB_APP_PRIVATE_KEY"} {
		if _, ok := sharedTrigger.Secrets[name]; !ok {
			t.Fatalf("shared review workflow does not declare secret %s", name)
		}
	}
	var tokenStep workflowStep
	for _, step := range sharedJob.Steps {
		if strings.HasPrefix(step.Uses, "actions/create-github-app-token@") {
			tokenStep = step
			break
		}
	}
	if tokenStep.Uses == "" {
		t.Fatal("shared review workflow does not mint an OpenShell GitHub App token")
	}
	if tokenStep.With["client-id"] != "${{ inputs.openshell-github-app-client-id }}" {
		t.Fatal("shared workflow does not use the GitHub App client ID input")
	}
	if tokenStep.With["private-key"] != "${{ secrets.OPENSHELL_GITHUB_APP_PRIVATE_KEY }}" {
		t.Fatal("shared workflow does not use the private-key secret")
	}
	for _, step := range sharedJob.Steps {
		if step.Env["GH_TOKEN"] == "${{ github.token }}" || step.Env["GITHUB_TOKEN"] == "${{ github.token }}" {
			t.Fatal("shared review workflow still uses the automatic workflow token")
		}
	}

	data, err := os.ReadFile("../tasks/github-pr-reviewer/workflow/opencode-harness.yaml")
	if err != nil {
		t.Fatal(err)
	}
	example := string(data)
	if !strings.Contains(example, `providers: ["${REVIEW_GITHUB_PROVIDER}"]`) {
		t.Fatal("review workflow does not attach the native GitHub provider")
	}
	if strings.Contains(example, "GITHUB_TOKEN") {
		t.Fatal("review workflow passes the GitHub token into the sandbox configuration")
	}
}

type workflowDocument struct {
	On   map[string]workflowTrigger `yaml:"on"`
	Jobs map[string]workflowJob     `yaml:"jobs"`
}

type workflowTrigger struct {
	Secrets map[string]workflowSecret `yaml:"secrets"`
}

type workflowSecret struct {
	Required bool `yaml:"required"`
}

type workflowJob struct {
	Uses    string            `yaml:"uses"`
	With    map[string]string `yaml:"with"`
	Secrets map[string]string `yaml:"secrets"`
	Steps   []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

func parseWorkflow(t *testing.T, data string) workflowDocument {
	t.Helper()
	var workflow workflowDocument
	if err := yaml.Unmarshal([]byte(data), &workflow); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	return workflow
}

func isSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
