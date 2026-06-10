package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/agent"
)

// writeStubCLI writes an executable shell script that records its arguments
// to argsFile (one invocation per line) and exits 0.
func writeStubCLI(t *testing.T, argsFile string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "openshell")
	script := "#!/bin/bash\necho \"$@\" >> " + argsFile + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestConfigureGateway_MissingCertsFallsBackInsecure(t *testing.T) {
	t.Setenv("OPENSHELL_GATEWAY_ENDPOINT", "")
	t.Setenv("OPENSHELL_GATEWAY_INSECURE", "")
	mtlsDir := t.TempDir() // no certs inside

	if err := configureGateway("https://gw.example:8080", mtlsDir, "openshell"); err != nil {
		t.Fatalf("configureGateway: %v", err)
	}
	if got := os.Getenv("OPENSHELL_GATEWAY_ENDPOINT"); got != "https://gw.example:8080" {
		t.Errorf("OPENSHELL_GATEWAY_ENDPOINT = %q", got)
	}
	if got := os.Getenv("OPENSHELL_GATEWAY_INSECURE"); got != "true" {
		t.Errorf("OPENSHELL_GATEWAY_INSECURE = %q, want true", got)
	}
}

func TestConfigureGateway_MTLS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	mtlsDir := t.TempDir()
	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if err := os.WriteFile(filepath.Join(mtlsDir, name), []byte("cert-"+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	argsFile := filepath.Join(t.TempDir(), "args.log")
	cli := writeStubCLI(t, argsFile)

	if err := configureGateway("https://gw.example:8080", mtlsDir, cli); err != nil {
		t.Fatalf("configureGateway: %v", err)
	}

	// gateway add must use the http endpoint (mTLS is configured via metadata).
	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(argsData), "gateway add http://gw.example:8080") {
		t.Errorf("cli args = %q, want 'gateway add http://...'", argsData)
	}

	gwDir := filepath.Join(home, ".config", "openshell", "gateways", "openshell")
	metaData, err := os.ReadFile(filepath.Join(gwDir, "metadata.json"))
	if err != nil {
		t.Fatalf("metadata.json not written: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("metadata.json invalid: %v", err)
	}
	if meta["gateway_endpoint"] != "https://gw.example:8080" {
		t.Errorf("gateway_endpoint = %v", meta["gateway_endpoint"])
	}
	if meta["auth_mode"] != "mtls" {
		t.Errorf("auth_mode = %v, want mtls", meta["auth_mode"])
	}

	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		data, err := os.ReadFile(filepath.Join(gwDir, "mtls", name))
		if err != nil {
			t.Fatalf("cert %s not copied: %v", name, err)
		}
		if string(data) != "cert-"+name {
			t.Errorf("cert %s content = %q", name, data)
		}
	}
}

func TestCheckProviders(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "openshell")
	// `provider get NAME` succeeds only for github.
	script := "#!/bin/bash\n[ \"$3\" = \"github\" ] && exit 0\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	registered := checkProviders([]string{"github", "atlassian"}, bin)
	if len(registered) != 1 || registered[0] != "github" {
		t.Errorf("registered = %v, want [github]", registered)
	}
}

func TestLaunchCreateSandbox_PassesAgentConfig(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.log")
	cli := writeStubCLI(t, argsFile)

	cfg := &agent.AgentConfig{
		Name:  "test-agent",
		Image: "ghcr.io/test:sandbox",
	}
	if err := launchCreateSandbox(cfg, []string{"github"}, "/tmp/payload", cli); err != nil {
		t.Fatalf("launchCreateSandbox: %v", err)
	}

	argsData, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(argsData)
	for _, want := range []string{
		"sandbox create",
		"--name test-agent",
		"--from ghcr.io/test:sandbox",
		"--provider github",
		"--upload /tmp/payload:/sandbox/.config",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("cli args missing %q in %q", want, args)
		}
	}
}
