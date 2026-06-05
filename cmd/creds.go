package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/robbycochran/harness-openshell/internal/k8s"
)

func ensureCreds(namespace string, force bool) error {
	ctx := context.Background()
	kc := k8s.New("", namespace)

	if !kc.NamespaceExists(ctx, namespace) {
		return fmt.Errorf("namespace '%s' not found — run: harness deploy --remote", namespace)
	}

	fmt.Println("=== Setting up cluster credentials ===")
	fmt.Printf("  Namespace: %s\n", namespace)
	fmt.Println()

	// GWS credentials
	fmt.Println("=== GWS ===")
	if force && kc.SecretExists(ctx, "openshell-gws") {
		kc.RunKubectl(ctx, "delete", "secret", "openshell-gws")
		fmt.Println("  Deleted existing secret.")
	}

	if kc.SecretExists(ctx, "openshell-gws") {
		fmt.Println("  openshell-gws: exists (use --force to recreate)")
	} else {
		if err := createGWSSecret(ctx, kc); err != nil {
			fmt.Printf("  GWS: %v\n", err)
		}
	}

	// Atlassian credentials
	fmt.Println()
	fmt.Println("=== Atlassian ===")
	if force && kc.SecretExists(ctx, "openshell-atlassian") {
		kc.RunKubectl(ctx, "delete", "secret", "openshell-atlassian")
		fmt.Println("  Deleted existing secret.")
	}

	if kc.SecretExists(ctx, "openshell-atlassian") {
		fmt.Println("  openshell-atlassian: exists (use --force to recreate)")
	} else if jiraURL := os.Getenv("JIRA_URL"); jiraURL != "" {
		jiraUser := os.Getenv("JIRA_USERNAME")
		_, err := kc.RunKubectl(ctx, "create", "secret", "generic", "openshell-atlassian",
			"--from-literal=JIRA_URL="+jiraURL,
			"--from-literal=JIRA_USERNAME="+jiraUser)
		if err != nil {
			return fmt.Errorf("creating atlassian secret: %w", err)
		}
		fmt.Printf("  openshell-atlassian: created (%s)\n", jiraURL)
	} else {
		fmt.Println("  Atlassian: JIRA_URL not set (skipping)")
	}

	return nil
}

func createGWSSecret(ctx context.Context, kc *k8s.Client) error {
	gwsPath, err := exec.LookPath("gws")
	if err != nil {
		fmt.Println("  GWS: not installed (skipping)")
		return nil
	}

	check := exec.Command(gwsPath, "auth", "status")
	check.Stdout = io.Discard
	check.Stderr = io.Discard
	if check.Run() != nil {
		fmt.Println("  GWS: not authenticated (run 'gws auth login')")
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
		fmt.Println("  GWS: export failed (skipping)")
		return nil
	}
	os.WriteFile(credFile, out, 0o600)

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
	fmt.Println("  openshell-gws: created")
	return nil
}
