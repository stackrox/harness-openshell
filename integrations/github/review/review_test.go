package review

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testBase = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const successOutput = "{\"type\":\"text\",\"part\":{\"text\":\"MODEL_OUTPUT\"}}\n{\"type\":\"step_finish\",\"part\":{\"reason\":\"stop\"}}\n"

func options(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", filepath.Join(root, "github-output"))
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(root, "step-summary"))
	skill, policy := filepath.Join(root, "SKILL.md"), filepath.Join(root, "policy.yaml")
	mustWrite(t, skill, "trusted review instructions")
	mustWrite(t, policy, "provider: ${REVIEW_GITHUB_PROVIDER}\npath: /repos/${REVIEW_REPOSITORY}/pulls/${REVIEW_PR}/comments\n")
	return Options{Dir: filepath.Join(root, "review"), Repository: "owner/repo", PR: 1, Head: testHead, Skill: skill, PolicyTemplate: policy, Workflow: "task.yaml", GitHubProvider: "github-review"}
}

func metadata(head, base string, labeled, draft bool) []byte {
	labels := []map[string]string{}
	if labeled {
		labels = append(labels, map[string]string{"name": "ai-review"})
	}
	data, _ := json.Marshal(map[string]any{"state": "open", "draft": draft, "labels": labels, "head": map[string]string{"sha": head}, "base": map[string]string{"sha": base}})
	return data
}

func goodAPI(_ context.Context, endpoint, _ string, _ int) ([]byte, error) {
	if strings.Contains(endpoint, "/compare/") {
		return []byte("diff data\n"), nil
	}
	return metadata(testHead, testBase, true, false), nil
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareEligibilityAndBoundedExactDiff(t *testing.T) {
	for _, scenario := range []string{"success", "unlabeled", "draft", "allow-draft", "stale-head", "oversized", "empty", "api-failure", "existing", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			o := options(t)
			compared := false
			s := Service{API: func(ctx context.Context, endpoint, accept string, limit int) ([]byte, error) {
				if scenario == "api-failure" {
					return nil, errors.New("offline")
				}
				if strings.Contains(endpoint, "/compare/") {
					compared = true
					if endpoint != "repos/owner/repo/compare/"+testBase+"..."+testHead || accept != "application/vnd.github.diff" || limit != MaxDiffBytes {
						t.Fatalf("wrong diff request %s %s %d", endpoint, accept, limit)
					}
					if scenario == "oversized" {
						return make([]byte, MaxDiffBytes+1), nil
					}
					if scenario == "empty" {
						return nil, nil
					}
					return goodAPI(ctx, endpoint, accept, limit)
				}
				return metadata(testHead, testBase, scenario != "unlabeled", scenario == "draft" || scenario == "allow-draft"), nil
			}}
			if scenario == "allow-draft" {
				o.AllowDrafts = true
			}
			if scenario == "stale-head" {
				o.Head = strings.Repeat("c", 40)
			}
			if scenario == "existing" {
				if err := os.Mkdir(o.Dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "symlink" {
				if err := os.Symlink(t.TempDir(), o.Dir); err != nil {
					t.Fatal(err)
				}
			}
			eligible, err := s.Prepare(t.Context(), o)
			wantSuccess := scenario == "success" || scenario == "allow-draft"
			wantSkip := scenario == "unlabeled" || scenario == "draft" || scenario == "stale-head"
			if eligible != wantSuccess || (err == nil) != (wantSuccess || wantSkip) {
				t.Fatalf("eligible=%v err=%v", eligible, err)
			}
			if wantSkip && compared {
				t.Fatal("ineligible PR fetched diff")
			}
			output, _ := os.ReadFile(os.Getenv("GITHUB_OUTPUT"))
			if strings.Contains(string(output), "eligible=true") != wantSuccess {
				t.Fatalf("eligibility output %q", output)
			}
		})
	}
}

func TestRunBoundary(t *testing.T) {
	for _, scenario := range []string{"success", "tampered", "wrong-repository", "wrong-pr", "wrong-head", "stale-before", "stale-base", "stale-after", "execution-failure", "cancel", "overflow", "malformed", "invalid-provider", "replay"} {
		t.Run(scenario, func(t *testing.T) {
			o := options(t)
			s := Service{API: goodAPI}
			if ok, err := s.Prepare(t.Context(), o); !ok || err != nil {
				t.Fatalf("prepare: %v", err)
			}
			ran := 0
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s.Execute = func(ctx context.Context, task Task, stdout, stderr io.Writer) error {
				ran++
				if task.Gateway != "" || task.Workspace != "" || task.Name == "ai-review" || len(task.Name) != 39 {
					t.Fatalf("target or name forced: %+v", task)
				}
				if task.Variables["REVIEW_HEAD"] != testHead || task.Variables["REVIEW_SKILL"] != o.Skill {
					t.Fatalf("wrong input: %+v", task)
				}
				if _, ok := task.Variables["GITHUB_TOKEN"]; ok {
					t.Fatal("credential forwarded")
				}
				policy, _ := os.ReadFile(task.Variables["REVIEW_POLICY"])
				if strings.Contains(string(policy), "${") || !strings.Contains(string(policy), "/repos/owner/repo/pulls/1/comments") {
					t.Fatalf("wrong policy: %s", policy)
				}
				_, _ = io.WriteString(stdout, successOutput)
				if scenario == "execution-failure" {
					return errors.New("sandbox cleanup failed")
				}
				if scenario == "cancel" {
					cancel()
					<-ctx.Done()
					return ctx.Err()
				}
				if scenario == "overflow" {
					_, _ = io.WriteString(stderr, strings.Repeat("x", MaxOutputBytes+1))
					return nil
				}
				if scenario == "malformed" {
					_, _ = io.WriteString(stdout, "{broken\n")
				}
				return nil
			}
			switch scenario {
			case "tampered":
				mustWrite(t, filepath.Join(o.Dir, "pr.diff"), "changed")
			case "wrong-repository":
				o.Repository = "owner/other"
			case "wrong-pr":
				o.PR = 2
			case "wrong-head":
				o.Head = strings.Repeat("c", 40)
			case "invalid-provider":
				o.GitHubProvider = "provider\nextra: rule"
			case "stale-before", "stale-base", "stale-after":
				s.API = func(context.Context, string, string, int) ([]byte, error) {
					base := testBase
					if scenario == "stale-base" {
						base = strings.Repeat("c", 40)
					}
					return metadata(testHead, base, scenario == "stale-base" || (scenario == "stale-after" && ran == 0), false), nil
				}
			}
			err := s.Run(ctx, o)
			wantSuccess := scenario == "success" || scenario == "replay" || strings.HasPrefix(scenario, "stale-")
			if (err == nil) != wantSuccess {
				t.Fatalf("run: %v", err)
			}
			if scenario == "replay" {
				if err := s.Run(ctx, o); err == nil || ran != 1 {
					t.Fatalf("replayed run: count=%d err=%v", ran, err)
				}
			}
			wantRun := scenario == "success" || scenario == "replay" || scenario == "stale-after" || scenario == "execution-failure" || scenario == "cancel" || scenario == "overflow" || scenario == "malformed"
			if (ran == 1) != wantRun {
				t.Fatalf("executions=%d", ran)
			}
			summary, _ := os.ReadFile(os.Getenv("GITHUB_STEP_SUMMARY"))
			if strings.Contains(string(summary), "MODEL_OUTPUT") || strings.Contains(string(summary), "AI review: prepared") {
				t.Fatalf("untrusted or premature summary: %s", summary)
			}
			if scenario != "replay" && strings.Count(string(summary), "## AI review:") != 1 {
				t.Fatalf("missing terminal summary %s", summary)
			}
			if scenario == "overflow" {
				info, err := os.Stat(filepath.Join(o.Dir, "agent.stderr"))
				if err != nil || info.Size() != MaxOutputBytes {
					t.Fatalf("unbounded diagnostics: %v %v", info, err)
				}
			}
		})
	}
}

func TestCallerSkillContainment(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "skill.md"), "skill")
	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWrite(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.md", outside, "link.md"} {
		if _, err := selectSkill(path, root); err == nil {
			t.Fatalf("accepted escaping skill %q", path)
		}
	}
	if _, err := selectSkill("skill.md", root); err != nil {
		t.Fatal(err)
	}
}
