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

// Local bootstrap remains a shell boundary around native OpenShell commands.
// Fakes verify ownership and ordering without a gateway or provider credentials.
func TestLocalReviewBootstrap(t *testing.T) {
	for _, scenario := range []string{"success", "workspace-failure", "profile-read-failure", "profile-import-failure", "existing-profile", "vertex-failure", "github-failure", "inference-failure", "review-failure", "cleanup-failure", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			for _, tool := range []string{"bash", "jq", "openssl"} {
				if _, err := exec.LookPath(tool); err != nil {
					t.Skipf("requires %s", tool)
				}
			}
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "scripts"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "review"), 0o700); err != nil {
				t.Fatal(err)
			}
			for path, data := range map[string][]byte{
				"scripts/pr-review-local.sh": mustRead(t, "../scripts/pr-review-local.sh"),
				"harness":                    []byte(fakeLocalReview), "openshell": []byte(fakeLocalReview),
				"timeout": []byte("#!/usr/bin/env bash\nshift\nexec \"$@\"\n"),
			} {
				if err := os.WriteFile(filepath.Join(root, path), data, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts/pr-review-local.sh"))
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "FAKE_SCENARIO="+scenario, "TRACE="+filepath.Join(root, "trace"), "READY="+filepath.Join(root, "ready"), "REVIEW_DIR="+filepath.Join(root, "review"), "GITHUB_TOKEN=synthetic-test-token", "GOOGLE_VERTEX_AI_TOKEN=synthetic-vertex-token", "VERTEX_AI_PROJECT_ID=test-project")
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
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
						t.Fatal("review never started")
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			err := cmd.Wait()
			if (err == nil) != (scenario == "success" || scenario == "existing-profile") {
				t.Fatalf("bootstrap error=%v\n%s", err, output.String())
			}
			trace := string(mustRead(t, filepath.Join(root, "trace")))
			if strings.Contains(trace, "sandbox delete") {
				t.Fatal("bootstrap took over runner cleanup")
			}
			workspaceDeleted := strings.Contains(trace, "workspace delete")
			if workspaceDeleted == (scenario == "workspace-failure") {
				t.Fatalf("incorrect workspace ownership:\n%s", trace)
			}
			if scenario == "profile-read-failure" && strings.Contains(trace, "profile import") {
				t.Fatal("failed read triggered profile mutation")
			}
			if scenario == "existing-profile" && strings.Contains(trace, "profile delete") {
				t.Fatal("deleted pre-existing profile")
			}
			if scenario == "vertex-failure" && strings.Contains(trace, "provider delete") {
				t.Fatal("deleted uncreated provider")
			}
			if scenario == "github-failure" {
				for _, line := range strings.Split(trace, "\n") {
					if strings.HasPrefix(line, "provider delete ") && strings.HasSuffix(line, " github-review") {
						t.Fatal("deleted uncreated github provider")
					}
				}
			}
			if strings.Contains(trace, "synthetic-test-token") || strings.Contains(trace, "synthetic-vertex-token") {
				t.Fatal("credential value entered command arguments")
			}
			if scenario == "cancel" && strings.Index(trace, "review stopped") > strings.Index(trace, "workspace delete") {
				t.Fatal("deleted workspace before runner stopped")
			}
		})
	}
}

const fakeLocalReview = `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$TRACE"
if [[ "${0##*/}" == harness ]]; then
  touch "$READY"
  if [[ "$FAKE_SCENARIO" == cancel ]]; then
    trap 'echo "review stopped" >> "$TRACE"; exit 143' TERM
    while :; do sleep 0.1; done
  fi
  [[ "$FAKE_SCENARIO" != review-failure ]]
  exit $?
fi
case "$1 ${2:-} ${3:-}" in
  'workspace create '*) [[ "$FAKE_SCENARIO" != workspace-failure ]] ;;
  'workspace delete '*) [[ "$FAKE_SCENARIO" != cleanup-failure ]] ;;
  'provider list-profiles '*)
    [[ "$FAKE_SCENARIO" != profile-read-failure ]] || exit 1
    if [[ "$FAKE_SCENARIO" == existing-profile ]]; then echo '[{"id":"github-review"}]';else echo '[]';fi ;;
  'provider profile import') [[ "$FAKE_SCENARIO" != profile-import-failure ]] ;;
  'provider create '*)
    if [[ "$*" == *'--name vertex-review '* && "$FAKE_SCENARIO" == vertex-failure ]];then exit 1;fi
    if [[ "$*" == *'--name github-review '* && "$FAKE_SCENARIO" == github-failure ]];then exit 1;fi ;;
  'inference set '*) [[ "$FAKE_SCENARIO" != inference-failure ]] ;;
esac
`
