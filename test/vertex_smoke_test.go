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

// Exercise the real orchestration script without credentials or a gateway.
func TestVertexSmoke(t *testing.T) {
	script, err := os.ReadFile("vertex-gemini-opencode.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "invalid", "agent-failure", "provider-failure", "cleanup-failure", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "test"), 0o700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string][]byte{
				"test/vertex-gemini-opencode.sh": script,
				"harness":                        []byte(fakeVertexCommand),
				"openshell":                      []byte(fakeVertexCommand),
			} {
				if err := os.WriteFile(filepath.Join(root, name), data, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", filepath.Join(root, "test/vertex-gemini-opencode.sh"))
			trace := filepath.Join(root, "trace")
			ready := filepath.Join(root, "ready")
			cmd.Env = append(os.Environ(), "SCENARIO="+scenario, "TRACE="+trace, "READY="+ready,
				"OPENSHELL_CLI="+filepath.Join(root, "openshell"), "GOOGLE_VERTEX_AI_TOKEN=fake",
				"VERTEX_AI_PROJECT_ID=test-project", "OPENSHELL_GATEWAY=test-gateway")
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if scenario == "cancel" {
				for {
					if _, err := os.Stat(ready); err == nil {
						if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
							t.Fatal(err)
						}
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
			err := cmd.Wait()
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected exit: %v\n%s", err, output.String())
			}
			if strings.Contains(output.String(), "RESULT: PASS") != (scenario == "success") {
				t.Fatalf("incorrect result: %s", output.String())
			}
			calls, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(calls), "workspace delete") || !strings.Contains(string(calls), "--sandboxes") {
				t.Fatalf("cleanup missing: %s", calls)
			}
			if strings.Contains(string(calls), "provider delete") == (scenario == "provider-failure") {
				t.Fatalf("provider cleanup must follow successful creation: %s", calls)
			}
		})
	}
}

const fakeVertexCommand = `#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$TRACE"
case "$1 ${2:-}" in
  'provider create') [[ "$SCENARIO" != provider-failure ]] ;;
  'workspace delete') [[ "$SCENARIO" != cleanup-failure ]] ;;
  'workflow apply '*)
    case "$SCENARIO" in
      cancel) touch "$READY"; trap 'exit 143' TERM; while :; do sleep 0.1; done ;;
      agent-failure) exit 42 ;;
      invalid) printf 'GEMINI_OPENCODE_OK\nunexpected trailing output\n' ;;
      *) printf 'GEMINI_OPENCODE_OK\n' ;;
    esac ;;
esac
`
