package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/internal/k8s"
	"github.com/robbycochran/harness-openshell/internal/status"
)

func ensureCreds(kc k8s.Runner, namespace string, force bool) error {
	ctx := context.Background()

	if !kc.NamespaceExists(ctx, namespace) {
		return fmt.Errorf("namespace '%s' not found — run: harness deploy --remote", namespace)
	}

	status.Section("Setting up cluster credentials")
	status.Detailf("Namespace: %s", namespace)

	// GWS credentials
	status.Section("GWS")
	if force && kc.SecretExists(ctx, "openshell-gws") {
		kc.RunKubectl(ctx, "delete", "secret", "openshell-gws")
		status.Info("Deleted existing secret")
	}

	if kc.SecretExists(ctx, "openshell-gws") {
		status.Info("openshell-gws: exists (use --force to recreate)")
	} else {
		if err := createGWSSecret(ctx, kc); err != nil {
			status.Failf("GWS: %v", err)
		}
	}

	// Atlassian credentials
	status.Section("Atlassian")
	if force && kc.SecretExists(ctx, "openshell-atlassian") {
		kc.RunKubectl(ctx, "delete", "secret", "openshell-atlassian")
		status.Info("Deleted existing secret")
	}

	if kc.SecretExists(ctx, "openshell-atlassian") {
		status.Info("openshell-atlassian: exists (use --force to recreate)")
	} else if jiraURL := os.Getenv("JIRA_URL"); jiraURL != "" {
		jiraUser := os.Getenv("JIRA_USERNAME")
		_, err := kc.RunKubectl(ctx, "create", "secret", "generic", "openshell-atlassian",
			"--from-literal=JIRA_URL="+jiraURL,
			"--from-literal=JIRA_USERNAME="+jiraUser)
		if err != nil {
			return fmt.Errorf("creating atlassian secret: %w", err)
		}
		status.OKf("openshell-atlassian: created (%s)", jiraURL)
	} else {
		status.Info("Atlassian: JIRA_URL not set (skipping)")
	}

	return nil
}

func createGWSSecret(ctx context.Context, kc k8s.Runner) error {
	gwsPath, err := exec.LookPath("gws")
	if err != nil {
		status.Info("GWS: not installed (skipping)")
		return nil
	}

	check := exec.Command(gwsPath, "auth", "status")
	check.Stdout = io.Discard
	check.Stderr = io.Discard
	if check.Run() != nil {
		status.Info("GWS: not authenticated (run 'gws auth login')")
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "harness-gws-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	credFile := filepath.Join(tmpDir, "credentials.json")
	out, err := exec.Command(gwsPath, "auth", "export", "--unmasked").Output()
	if err != nil {
		status.Info("GWS: export failed (skipping)")
		return nil
	}
	if err := os.WriteFile(credFile, out, 0o600); err != nil {
		return fmt.Errorf("writing gws credentials: %w", err)
	}

	args := []string{"create", "secret", "generic", "openshell-gws",
		"--from-file=credentials.json=" + credFile}

	gwsConfigDir := os.Getenv("GWS_CONFIG_DIR")
	if gwsConfigDir == "" {
		home, _ := os.UserHomeDir()
		gwsConfigDir = filepath.Join(home, ".config", "gws")
	}
	clientSecret := filepath.Join(gwsConfigDir, "client_secret.json")
	if _, err := os.Stat(clientSecret); err == nil {
		args = append(args, "--from-file=client_secret.json="+clientSecret)
	}

	if _, err := kc.RunKubectl(ctx, args...); err != nil {
		return fmt.Errorf("creating gws secret: %w", err)
	}
	status.OK("openshell-gws: created")
	return nil
}
