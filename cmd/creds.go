package cmd

import (
	"context"
	"fmt"
	"os"

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
