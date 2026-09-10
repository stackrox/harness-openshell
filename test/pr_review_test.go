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
	for _, scenario := range []string{"success", "unlabeled", "stale", "oversized", "tampered", "agent-failure", "provider-failure", "cleanup-failure", "cancel", "truncated", "incomplete", "empty", "error", "tool_use", "tool_exit", "tool_missing_exit"} {
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
			if (err == nil) != (scenario == "success" || scenario == "stale") {
				t.Fatalf("unexpected result: %v\n%s", err, logs.String())
			}
			trace, _ := os.ReadFile(filepath.Join(root, "trace"))
			if scenario == "tampered" {
				if strings.Contains(string(trace), "workspace create") {
					t.Fatal("changed diff reached workspace creation")
				}
				return
			}
			for _, action := range []string{"--sandboxes", "workspace delete"} {
				if !strings.Contains(string(trace), action) {
					t.Fatalf("missing cleanup: %s", trace)
				}
			}
			if strings.Contains(string(trace), "provider delete") == (scenario == "provider-failure") {
				t.Fatal("provider cleanup must follow creation")
			}
			summary, _ := os.ReadFile(filepath.Join(root, "review/summary.md"))
			if strings.Contains(string(summary), "AI review: completed") != (scenario == "success") || strings.Contains(string(summary), "MODEL_OUTPUT") {
				t.Fatalf("incorrect or model-controlled summary: %s", summary)
			}
		})
	}
}

const fakeReviewCommand = `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$TRACE"
if [[ "${0##*/}" == gh ]]; then
  if [[ "$2" == */compare/* ]]; then
    if [[ "$FAKE_SCENARIO" == oversized ]]; then head -c 204801 /dev/zero; else printf 'diff data\n'; fi
  else
    labels='[{"name":"ai-review"}]'
    [[ "$FAKE_SCENARIO" != unlabeled ]] || labels='[]'
    [[ "$FAKE_SCENARIO" != stale || ! -f "$READY" ]] || labels='[]'
    printf '{"state":"open","draft":false,"labels":%s,"head":{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"base":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}\n' "$labels"
  fi
  exit 0
fi
case "$1 ${2:-}" in
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
      error|tool_use) printf '{"type":"%s"}\n' "$FAKE_SCENARIO" ;;
      tool_exit) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{"exit":7},"output":"ordinary command failed"}}}' ;;
      tool_missing_exit) printf '%s\n' '{"type":"tool_use","part":{"state":{"status":"completed","metadata":{},"output":"missing exit"}}}' ;;
      *) printf '%s\n' '{"type":"step_finish","part":{"reason":"stop"}}' ;;
    esac ;;
esac
`
