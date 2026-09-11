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
)

func TestPRReview(t *testing.T) {
	script, err := os.ReadFile("../scripts/pr-review.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "unlabeled", "stale", "oversized", "tampered", "agent-failure", "provider-failure", "cleanup-failure", "sandbox-gone", "cancel", "truncated", "malformed-trailing", "incomplete", "empty", "error", "tool_use", "tool_exit", "tool_missing_exit", "unrelated-422", "unrelated-422-line", "unrelated-422-comment", "unrelated-comment", "comment-position"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			stepSummary := filepath.Join(root, "step-summary")
			t.Cleanup(func() {
				data, err := os.ReadFile(stepSummary)
				if err != nil || strings.Count(string(data), "## AI review:") != 1 || strings.Contains(string(data), "AI review: prepared") {
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
			for name, data := range map[string][]byte{"scripts/pr-review.sh": script, "scripts/review/validate-agent-output.sh": validator, "harness": []byte(fakeReviewCommand), "openshell": []byte(fakeReviewCommand), "gh": []byte(fakeReviewCommand), "review-policy.yaml": []byte("version: 1\nnetwork_policies: {}\n"), "output": nil, "step-summary": nil} {
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
				"GITHUB_STEP_SUMMARY="+stepSummary, "GOOGLE_VERTEX_AI_TOKEN=fake", "VERTEX_AI_PROJECT_ID=test-project", "GITHUB_TOKEN=fake", "REVIEW_POLICY_TEMPLATE="+filepath.Join(root, "review-policy.yaml"))
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
			cmd.Env = prepare.Env
			var logs bytes.Buffer
			cmd.Stdout, cmd.Stderr = &logs, &logs
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if scenario == "cancel" {
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
			if (err == nil) != (scenario == "success" || scenario == "stale" || scenario == "sandbox-gone" || scenario == "comment-position") {
				t.Fatalf("unexpected result: %v\n%s", err, logs.String())
			}
			trace, _ := os.ReadFile(filepath.Join(root, "trace"))
			if scenario == "tampered" {
				if strings.Contains(string(trace), "workspace create") {
					t.Fatal("changed diff reached workspace creation")
				}
				return
			}
			for _, action := range []string{"sandbox delete", "workspace delete"} {
				if !strings.Contains(string(trace), action) {
					t.Fatalf("missing cleanup: %s", trace)
				}
			}
			if strings.Contains(string(trace), "provider delete") == (scenario == "provider-failure") {
				t.Fatal("provider cleanup must follow creation")
			}
			summary, _ := os.ReadFile(filepath.Join(root, "review/summary.md"))
			if strings.Contains(string(summary), "AI review: completed") != (scenario == "success" || scenario == "sandbox-gone" || scenario == "comment-position") || strings.Contains(string(summary), "MODEL_OUTPUT") {
				t.Fatalf("incorrect or model-controlled summary: %s", summary)
			}
		})
	}
}

func TestGitHubAppTokenIsHostOnly(t *testing.T) {
	script, err := os.ReadFile("../scripts/pr-review.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `REVIEW_SKILL="${REVIEW_SKILL:-skills/pr-review/SKILL.md}"`) {
		t.Fatal("review wrapper default skill path is not relative to the workflow file")
	}

	caller := string(mustRead(t, "../.github/workflows/ai-review.yml"))
	const usesPrefix = "    uses: stackrox/harness-openshell/.github/workflows/pr-review-reusable.yml@"
	var pinnedRef string
	for _, line := range strings.Split(caller, "\n") {
		if strings.HasPrefix(line, usesPrefix) {
			pinnedRef = strings.TrimPrefix(line, usesPrefix)
			break
		}
	}
	if !isSHA(pinnedRef) {
		t.Fatalf("caller does not pin the shared review workflow to a commit: %q", pinnedRef)
	}
	if !strings.Contains(caller, "harness-ref: "+pinnedRef) {
		t.Fatalf("caller harness-ref does not match workflow pin %q", pinnedRef)
	}
	for _, required := range []string{
		"openshell-github-app-client-id: ${{ vars.OPENSHELL_GITHUB_APP_CLIENT_ID }}",
		"secrets: inherit",
	} {
		if !strings.Contains(caller, required) {
			t.Fatalf("caller is missing %q", required)
		}
	}
	if strings.Contains(caller, "actions/create-github-app-token@") || strings.Contains(caller, "scripts/pr-review.sh") {
		t.Fatal("caller still contains shared review implementation")
	}

	shared := string(mustRead(t, "../.github/workflows/pr-review-reusable.yml"))
	for _, required := range []string{
		"workflow_call:",
		"actions/create-github-app-token@",
		"client-id: ${{ inputs.openshell-github-app-client-id }}",
		"private-key: ${{ secrets.OPENSHELL_GITHUB_APP_PRIVATE_KEY }}",
	} {
		if !strings.Contains(shared, required) {
			t.Fatalf("shared review workflow is missing %q", required)
		}
	}
	if strings.Contains(shared, "GH_TOKEN: ${{ github.token }}") || strings.Contains(shared, "GITHUB_TOKEN: ${{ github.token }}") {
		t.Fatal("shared review workflow still uses the automatic workflow token")
	}

	data, err := os.ReadFile("../examples/github-pr-reviewer/opencode-harness.yaml")
	if err != nil {
		t.Fatal(err)
	}
	example := string(data)
	if !strings.Contains(example, "providers: [github-review]") {
		t.Fatal("review workflow does not attach the native GitHub provider")
	}
	if strings.Contains(example, "GITHUB_TOKEN") {
		t.Fatal("review workflow passes the GitHub token into the sandbox configuration")
	}
}

func isSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
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
  'sandbox delete') [[ "$FAKE_SCENARIO" != sandbox-gone ]] || { echo 'sandbox not found' >&2; exit 1; } ;;
  'provider create') [[ "$FAKE_SCENARIO" != provider-failure ]] ;;
  'workspace delete') [[ "$FAKE_SCENARIO" != cleanup-failure ]] ;;
  'workflow apply')
    touch "$READY"
    printf 'diagnostic without trailing newline' >&2
    case "$FAKE_SCENARIO" in
      cancel) trap 'exit 143' TERM; while :; do sleep 0.1; done ;;
      agent-failure) exit 42 ;;
    esac
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
      unrelated-422) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"unrelated build failed at record 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-422-line) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"build failed at line 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-422-comment) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment delivery failed with status 422"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      unrelated-comment) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment formatting failed"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      comment-position) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":1},"output":"comment position is invalid"}}}'; printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
      *) printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
    esac ;;
esac
`
