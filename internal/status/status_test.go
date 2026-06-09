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

func TestShowEquivalentCmd_OnlyWhenEnabled(t *testing.T) {
	ShowCommands = false
	out := captureStdout(func() {
		ShowEquivalentCmd("openshell", "sandbox", "list")
	})
	if out != "" {
		t.Errorf("expected no output when ShowCommands=false, got: %q", out)
	}
}

func TestShowEquivalentCmd_Prints(t *testing.T) {
	ShowCommands = true
	defer func() { ShowCommands = false }()
	out := captureStdout(func() {
		ShowEquivalentCmd("openshell", "sandbox", "list")
	})
	if !strings.Contains(out, "$ openshell sandbox list") {
		t.Errorf("expected equivalent command, got: %q", out)
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

func TestTable_Alignment(t *testing.T) {
	out := captureStdout(func() {
		Table(
			[]string{"NAME", "PHASE"},
			[][]string{
				{"my-agent", "Ready"},
				{"test", "Stopped"},
			},
		)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header + separator + 2 rows), got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Errorf("header missing NAME: %q", lines[0])
	}
	if !strings.Contains(lines[1], "─") {
		t.Errorf("separator missing: %q", lines[1])
	}
	if !strings.Contains(lines[2], "my-agent") {
		t.Errorf("row 1 missing: %q", lines[2])
	}
}

func TestTable_Empty(t *testing.T) {
	out := captureStdout(func() {
		Table([]string{"NAME", "PHASE"}, nil)
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + separator), got %d: %q", len(lines), out)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
