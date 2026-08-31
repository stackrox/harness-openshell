package status

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func captureCmd(name string, args ...string) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	Verbose = true
	ShowCommands = false
	Cmd(name, args...)
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestCmdRedactsCredential(t *testing.T) {
	out := captureCmd("openshell", "provider", "create", "github", "--credential", "GITHUB_TOKEN=ghp_secret123")
	if got := out; got == "" {
		t.Fatal("expected output")
	}
	if contains(out, "ghp_secret123") {
		t.Errorf("credential value leaked: %s", out)
	}
	if !contains(out, "GITHUB_TOKEN=***") {
		t.Errorf("expected redacted credential, got: %s", out)
	}
}

func TestCmdRedactsMultipleCredentials(t *testing.T) {
	out := captureCmd("openshell", "provider", "create", "atlassian",
		"--credential", "JIRA_API_TOKEN=secret1",
		"--credential", "JIRA_URL=https://example.com")
	if contains(out, "secret1") {
		t.Errorf("first credential leaked: %s", out)
	}
	if contains(out, "https://example.com") {
		t.Errorf("second credential leaked: %s", out)
	}
	if !contains(out, "JIRA_API_TOKEN=***") {
		t.Errorf("expected redacted JIRA_API_TOKEN, got: %s", out)
	}
	if !contains(out, "JIRA_URL=***") {
		t.Errorf("expected redacted JIRA_URL, got: %s", out)
	}
}

func TestCmdRedactsFromLiteral(t *testing.T) {
	out := captureCmd("kubectl", "create", "secret", "generic", "openshell-atlassian",
		"--from-literal=JIRA_API_TOKEN=mytoken",
		"--from-literal=JIRA_URL=https://example.com")
	if contains(out, "mytoken") {
		t.Errorf("token leaked: %s", out)
	}
	if !contains(out, "--from-literal=JIRA_API_TOKEN=***") {
		t.Errorf("expected redacted token, got: %s", out)
	}
	// JIRA_URL doesn't match sensitive keywords, should pass through
	if !contains(out, "--from-literal=JIRA_URL=https://example.com") {
		t.Errorf("non-sensitive literal should not be redacted, got: %s", out)
	}
}

func TestCmdDoesNotRedactNonSensitiveLiteral(t *testing.T) {
	out := captureCmd("kubectl", "create", "configmap", "test",
		"--from-literal=JIRA_URL=https://example.com",
		"--from-literal=NAMESPACE=openshell")
	if !contains(out, "JIRA_URL=https://example.com") {
		t.Errorf("non-sensitive literal was redacted: %s", out)
	}
	if !contains(out, "NAMESPACE=openshell") {
		t.Errorf("non-sensitive literal was redacted: %s", out)
	}
}

func TestCmdRedactsSensitiveEnv(t *testing.T) {
	// sandbox create --env carries secrets on the plaintext path; a sensitive
	// key's value must not leak into diagnostics, but the key stays visible.
	out := captureCmd("openshell", "sandbox", "create",
		"--env", "ANTHROPIC_API_KEY=sk-secret-xyz",
		"--env", "ANTHROPIC_BASE_URL=https://inference.local")
	if contains(out, "sk-secret-xyz") {
		t.Errorf("sensitive env value leaked: %s", out)
	}
	if !contains(out, "ANTHROPIC_API_KEY=***") {
		t.Errorf("expected redacted sensitive env, got: %s", out)
	}
	// benign env (no sensitive keyword) stays readable for debugging.
	if !contains(out, "ANTHROPIC_BASE_URL=https://inference.local") {
		t.Errorf("benign env should not be redacted, got: %s", out)
	}
}

func TestCmdEnvKeyOnly(t *testing.T) {
	// --env KEY (no =VALUE) passes through unchanged.
	out := captureCmd("openshell", "sandbox", "create", "--env", "ANTHROPIC_API_KEY")
	if !contains(out, "ANTHROPIC_API_KEY") {
		t.Errorf("env key should be preserved: %s", out)
	}
}

func TestCmdCredentialKeyOnly(t *testing.T) {
	// --credential KEY (no =VALUE) should pass through as-is
	out := captureCmd("openshell", "provider", "create", "github", "--credential", "GITHUB_TOKEN")
	if !contains(out, "GITHUB_TOKEN") {
		t.Errorf("credential key should be preserved: %s", out)
	}
}

func TestCmdNotVerbose(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	Verbose = false
	ShowCommands = false
	Cmd("openshell", "--credential", "TOKEN=secret")
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.Len() > 0 {
		t.Errorf("expected no output when not verbose, got: %s", buf.String())
	}
}

func TestCmdNormalArgs(t *testing.T) {
	out := captureCmd("openshell", "sandbox", "create", "--from", "image:latest", "--provider", "github")
	expected := "  $ openshell sandbox create --from image:latest --provider github\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestCmdShowCommands_PrintsToStdout(t *testing.T) {
	Verbose = false
	ShowCommands = true
	defer func() { ShowCommands = false }()
	out := captureStdout(func() {
		Cmd("openshell", "sandbox", "create", "--name", "test")
	})
	if !strings.Contains(out, "$ openshell sandbox create --name test") {
		t.Errorf("expected command on stdout, got: %q", out)
	}
}

func TestCmdShowCommands_RedactsCredentials(t *testing.T) {
	Verbose = false
	ShowCommands = true
	defer func() { ShowCommands = false }()
	out := captureStdout(func() {
		Cmd("openshell", "provider", "create", "github", "--credential", "TOKEN=secret")
	})
	if contains(out, "secret") {
		t.Errorf("credential leaked in show-commands: %s", out)
	}
	if !contains(out, "TOKEN=***") {
		t.Errorf("expected redacted credential: %s", out)
	}
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestHeader(t *testing.T) {
	out := captureStdout(func() { Header("Sandboxes") })
	if !strings.Contains(out, "Sandboxes") {
		t.Errorf("missing title: %q", out)
	}
	if !strings.Contains(out, "─") {
		t.Errorf("missing underline: %q", out)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
