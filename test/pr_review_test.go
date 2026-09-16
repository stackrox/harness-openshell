package test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestPRReview(t *testing.T) {
	script, err := os.ReadFile("../scripts/pr-review.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "unlabeled", "stale", "oversized", "tampered", "agent-failure", "provider-failure", "cleanup-failure", "local-success", "local-cancel", "partial-provider-failure", "workspace-failure", "profile-read-failure", "existing-profile", "status-failure", "cancel", "truncated", "malformed-trailing", "incomplete", "empty", "error", "tool_use", "tool_exit", "tool_missing_exit", "read-tool", "tool_recovered", "unrelated-422", "unrelated-422-line", "unrelated-422-comment", "unrelated-comment", "success-then-failure", "comment-position"} {
		t.Run(scenario, func(t *testing.T) {
			local := scenario == "provider-failure" || scenario == "cleanup-failure" || scenario == "local-success" || scenario == "local-cancel" || scenario == "partial-provider-failure" || scenario == "workspace-failure" || scenario == "profile-read-failure" || scenario == "existing-profile"
			root := t.TempDir()
			stepSummary := filepath.Join(root, "step-summary")
			t.Cleanup(func() {
				data, err := os.ReadFile(stepSummary)
				if !local && (err != nil || strings.Count(string(data), "## AI review:") != 1 || strings.Contains(string(data), "AI review: prepared")) {
					t.Errorf("expected exactly one terminal step summary: %v\n%s", err, data)
				}
			})
			if err := os.Mkdir(filepath.Join(root, "scripts"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "scripts", "review"), 0o700); err != nil {
				t.Fatal(err)
			}
			validator, err := os.ReadFile("../scripts/review/validate-agent-output.sh")
			if err != nil {
				t.Fatal(err)
			}
			validatorInfo, err := os.Stat("../scripts/review/validate-agent-output.sh")
			if err != nil {
				t.Fatal(err)
			}
			validatorMode := validatorInfo.Mode().Perm()
			if validatorMode&0o111 == 0 {
				t.Fatalf("validator must be executable: mode %o", validatorMode)
			}
			for name, data := range map[string][]byte{"scripts/pr-review.sh": script, "scripts/pr-review-local.sh": mustRead(t, "../scripts/pr-review-local.sh"), "scripts/review/validate-agent-output.sh": validator, "harness": []byte(fakeReviewCommand), "openshell": []byte(fakeReviewCommand), "gh": []byte(fakeReviewCommand), "review-policy.yaml": []byte("version: 1\nnetwork_policies: {}\n"), "output": nil, "step-summary": nil} {
				mode := os.FileMode(0o700)
				if name == "scripts/review/validate-agent-output.sh" {
					mode = validatorMode
				}
				if err := os.WriteFile(filepath.Join(root, name), data, mode); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			prepare := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts/pr-review.sh"), "prepare")
			prepare.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"),
				"FAKE_SCENARIO="+scenario, "TRACE="+filepath.Join(root, "trace"), "READY="+filepath.Join(root, "ready"),
				"REVIEW_DIR="+filepath.Join(root, "review"), "REVIEW_REPOSITORY=owner/repo", "REVIEW_PR=1", "REVIEW_HEAD=", "GITHUB_OUTPUT="+filepath.Join(root, "output"),
				"GITHUB_STEP_SUMMARY="+stepSummary, "GOOGLE_VERTEX_AI_TOKEN=", "VERTEX_AI_PROJECT_ID=", "GITHUB_TOKEN=", "OPENSHELL_GATEWAY=managed-test", "OPENSHELL_WORKSPACE=shared-test", "REVIEW_POLICY_TEMPLATE="+filepath.Join(root, "review-policy.yaml"))
			out, err := prepare.CombinedOutput()
			if scenario == "oversized" {
				if err == nil {
					info, _ := os.Stat(filepath.Join(root, "review/pr.diff"))
					t.Fatalf("oversized diff accepted: %v\n%s", info, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("prepare: %v\n%s", err, out)
			}
			if scenario == "unlabeled" {
				output, _ := os.ReadFile(filepath.Join(root, "output"))
				if len(output) != 0 {
					t.Fatal("unlabeled PR enabled")
				}
				return
			}
			if scenario == "tampered" {
				if err := os.WriteFile(filepath.Join(root, "review/pr.diff"), []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts/pr-review.sh"), "run")
			if local {
				cmd = exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts/pr-review-local.sh"))
			}
			cmd.Env = prepare.Env
			if local {
				cmd.Env = append(cmd.Env, "GOOGLE_VERTEX_AI_TOKEN=fake", "VERTEX_AI_PROJECT_ID=test-project", "GITHUB_TOKEN=fake")
			}
			var logs bytes.Buffer
			cmd.Stdout, cmd.Stderr = &logs, &logs
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if scenario == "cancel" || scenario == "local-cancel" {
				for {
					if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
						_ = cmd.Process.Signal(syscall.SIGTERM)
						break
					}
					select {
					case <-ctx.Done():
						_ = cmd.Wait()
						t.Fatal("agent did not start")
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			err = cmd.Wait()
			if (err == nil) != (scenario == "success" || scenario == "stale" || scenario == "local-success" || scenario == "existing-profile" || scenario == "comment-position" || scenario == "read-tool" || scenario == "tool_recovered") {
				t.Fatalf("unexpected result: %v\n%s", err, logs.String())
			}
			trace, _ := os.ReadFile(filepath.Join(root, "trace"))
			if scenario == "tampered" {
				if strings.Contains(string(trace), "workflow apply") {
					t.Fatal("changed diff reached the runner")
				}
				return
			}
			if strings.Contains(string(trace), "sandbox delete") {
				t.Fatal("review wrapper must leave sandbox cleanup to the runner")
			}
			if !local {
				for _, action := range []string{"workspace ", "provider ", "inference ", "--gateway", "--workspace"} {
					if strings.Contains(string(trace), action) {
						t.Fatalf("existing-target review performed setup or forced a target: %s", trace)
					}
				}
				if !strings.Contains(string(trace), "target managed-test shared-test") {
					t.Fatal("configured target environment was not preserved")
				}
			} else {
				if strings.Contains(string(trace), "workspace delete") == (scenario == "workspace-failure") {
					t.Fatalf("incorrect workspace cleanup: %s", trace)
				}
				if scenario == "provider-failure" && strings.Contains(string(trace), "provider delete") {
					t.Fatal("deleted a provider that was not created")
				}
				if scenario == "existing-profile" && strings.Contains(string(trace), "provider profile delete") {
					t.Fatal("deleted an existing profile")
				}
				if scenario == "profile-read-failure" && strings.Contains(string(trace), "provider profile import") {
					t.Fatal("profile read failure triggered an import")
				}
				if scenario == "partial-provider-failure" {
					if !strings.Contains(string(trace), "provider delete") {
						t.Fatal("missing cleanup of created Vertex provider")
					}
					for _, line := range strings.Split(string(trace), "\n") {
						if strings.HasPrefix(line, "provider delete ") && strings.HasSuffix(line, " github-review") {
							t.Fatal("deleted uncreated GitHub provider")
						}
					}
				}
				if scenario == "local-cancel" && (!strings.Contains(string(trace), "runner stopped") || strings.Index(string(trace), "runner stopped") > strings.Index(string(trace), "workspace delete")) {
					t.Fatal("workspace deleted before runner stopped")
				}
			}
			summary, _ := os.ReadFile(filepath.Join(root, "review/summary.md"))
			if strings.Contains(string(summary), "AI review: completed") != (scenario == "success" || scenario == "local-success" || scenario == "existing-profile" || scenario == "cleanup-failure" || scenario == "comment-position" || scenario == "read-tool" || scenario == "tool_recovered") || strings.Contains(string(summary), "MODEL_OUTPUT") {
				t.Fatalf("incorrect or model-controlled summary: %s", summary)
			}
		})
	}
}

func TestGitHubAppTokenIsHostOnly(t *testing.T) {
	script, err := os.ReadFile("../scripts/pr-review-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "--name github-review --type github-review --credential GITHUB_TOKEN") {
		t.Fatal("review wrapper does not use the endpointless github-review provider profile")
	}
	if !strings.Contains(string(script), "provider profile import") ||
		!strings.Contains(string(script), "tasks/github-pr-reviewer/openshell/providers/github-review.yaml") {
		t.Fatal("review wrapper does not bootstrap the endpointless github-review profile")
	}
	if !strings.Contains(string(script), "provider profile delete") {
		t.Fatal("review wrapper does not clean up an imported provider profile")
	}
	if !strings.Contains(string(mustRead(t, "../scripts/pr-review.sh")), `REVIEW_SKILL="${REVIEW_SKILL:-$PWD/tasks/github-pr-reviewer/workflow/skills/pr-review/SKILL.md}"`) {
		t.Fatal("review wrapper default skill must resolve from the trusted checkout")
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
	if input, ok := sharedTrigger.Inputs["review-label"]; !ok || input.Default != "ai-review" {
		t.Fatalf("shared review workflow default label = %#v, want ai-review", input)
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
	var task map[string]any
	if err := yaml.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}
	if _, ok := task["inference"]; ok {
		t.Fatal("review task must consume the route owned by setup")
	}
	example := string(data)
	if !strings.Contains(example, "providers: [github-review]") {
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
	Inputs  map[string]workflowInput  `yaml:"inputs"`
	Secrets map[string]workflowSecret `yaml:"secrets"`
}

type workflowInput struct {
	Default string `yaml:"default"`
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

const fakeReviewCommand = `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$TRACE"
assert_name_length() {
  local name="" next
  for ((i = 1; i <= $#; i++)); do
    if [[ "${!i}" == --name ]]; then
      next=$((i + 1))
      ((next <= $#)) || return 1
      name="${!next}"
    fi
  done
  [[ -n "$name" && ${#name} -le 19 ]]
}
if [[ "${0##*/}" == gh ]]; then
  if [[ "$2" == */compare/* ]]; then
    if [[ "$FAKE_SCENARIO" == oversized ]]; then head -c 262145 /dev/zero; else printf 'diff data\n'; fi
  else
    labels='[{"name":"ai-review"}]'
    [[ "$FAKE_SCENARIO" != unlabeled ]] || labels='[]'
    [[ "$FAKE_SCENARIO" != stale || ! -f "$READY" ]] || labels='[]'
    printf '{"state":"open","draft":false,"labels":%s,"head":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"base":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}\n' "$labels"
  fi
  exit 0
fi
case "$1 ${2:-}" in
  'workspace create') assert_name_length "$@" && [[ "$FAKE_SCENARIO" != workspace-failure ]] ;;
  'provider list-profiles')
    [[ "$FAKE_SCENARIO" != profile-read-failure ]] || exit 1
    if [[ "$FAKE_SCENARIO" == existing-profile ]]; then echo '[{"id":"github-review"}]'; else echo '[]'; fi ;;
  'provider create')
    [[ "$FAKE_SCENARIO" != provider-failure ]] || exit 1
    if [[ "$FAKE_SCENARIO" == partial-provider-failure && "$*" == *'--name github-review '* ]]; then exit 1; fi ;;
  'workspace delete') [[ "$FAKE_SCENARIO" != cleanup-failure ]] ;;
  'workflow apply')
    assert_name_length "$@" || exit 1
    printf 'target %s %s\n' "${OPENSHELL_GATEWAY:-}" "${OPENSHELL_WORKSPACE:-}" >> "$TRACE"
    touch "$READY"
    printf 'diagnostic without trailing newline' >&2
    case "$FAKE_SCENARIO" in
      cancel|local-cancel) trap 'echo "runner stopped" >> "$TRACE"; exit 143' TERM; while :; do sleep 0.1; done ;;
      agent-failure) exit 42 ;;
    esac
    result_file=""
    for ((i = 1; i <= $#; i++)); do
      if [[ "${!i}" == --result-file ]]; then
        next=$((i + 1))
        ((next <= $#)) && result_file="${!next}"
      fi
    done
    if [[ -n "$result_file" ]]; then
      printf '%s\n' '{"status":"succeeded","phase":"complete"}' > "$result_file"
    fi
    printf '%s\n' 'harness status'
    if [[ "$FAKE_SCENARIO" == empty ]]; then
      printf '%s\n' '{"type":"text","part":{"text":" "}}'
    else
      printf '%s\n' '{"type":"text","part":{"text":"MODEL_OUTPUT"}}'
    fi
    case "$FAKE_SCENARIO" in
      incomplete) exit 0 ;;
      truncated) printf '%s\n' '{"type":"step_finish","part":{"reason":"length"}}' ;;
      malformed-trailing) printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}'; printf '%s\n' '{"type":"error",';;
      error|tool_use) printf '{"type":"%s"}\n' "$FAKE_SCENARIO" ;;
      tool_exit) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":7},"output":"ordinary command failed"}}}' ;;
      tool_missing_exit) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{},"output":"missing exit"}}}' ;;
      read-tool) printf '%s\n' '{"type":"tool_use","part":{"tool":"read","state":{"status":"completed","metadata":{},"output":"read succeeded"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      tool_recovered) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":2},"output":"unexpected EOF while looking for matching quote"}}}'; printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":0},"output":"retry succeeded"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}';;
      unrelated-422) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"unrelated build failed at record 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-422-line) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"build failed at line 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-422-comment) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment delivery failed with status 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-comment) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment formatting failed"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      success-then-failure) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":0},"output":"unrelated success"}}}'; printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"ordinary command failed"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      comment-position) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment position is invalid"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      *) printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
    esac
    [[ "$FAKE_SCENARIO" != status-failure ]] || exit 42 ;;
esac
`
