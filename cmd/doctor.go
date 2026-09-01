package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

type CheckResult struct {
	Group   string `json:"group"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func NewDoctorCmd(defaultCfg []byte, newClient openshell.Factory) *cobra.Command {
	var (
		file   string
		output string
	)
	// Assigned by registerTargetFlags below; RunE reads them at execution time.
	var gatewayName, workspace *string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate environment for configured sandbox",
		Long: `Check that the configured gateway is reachable and that every referenced
provider is registered. Provider credentials are owned by platform bootstrap and
are never loaded by the harness.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseOutputFormat(output)
			if err != nil {
				return err
			}

			var h *config.Harness
			var target openshell.Target
			if file != "" {
				workflow, err := loadWorkflow(file, *gatewayName, *workspace, applyOverrides{})
				if err != nil {
					return err
				}
				h = workflow.Desired
				target = workflow.Target
			} else {
				h, err = config.Parse(defaultCfg)
				if err != nil {
					return fmt.Errorf("parsing default config: %w", err)
				}
				h, err = config.Resolve(h, os.Getenv)
				if err != nil {
					return fmt.Errorf("resolving default config: %w", err)
				}
				target = openshell.ResolveTarget(*gatewayName, *workspace, h.Spec.Target.Gateway, h.Spec.Target.Workspace, os.Getenv)
			}

			providers := configuredProviders(h)
			providerNames := make([]string, len(providers))
			for i, provider := range providers {
				providerNames[i] = provider.Name
			}
			results := runOnlineChecks(cmd.Context(), newClient, target, providerNames)

			if format != formatTable {
				if err := printStructured(format, results); err != nil {
					return err
				}
				return doctorResultError(results)
			}

			printDoctorTable(results)
			return doctorResultError(results)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to harness YAML")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (table, json, yaml)")
	gatewayName, workspace = registerTargetFlags(cmd)

	return cmd
}

func doctorResultError(results []CheckResult) error {
	for _, result := range results {
		if result.Status == "fail" {
			return fmt.Errorf("one or more checks failed")
		}
	}
	return nil
}

type configuredProvider struct {
	Name string
}

func configuredProviders(cfg *config.Harness) []configuredProvider {
	providers := make([]configuredProvider, 0, len(cfg.Spec.Providers)+len(cfg.Spec.Sandbox.Providers))
	seen := make(map[string]struct{}, cap(providers))
	for _, provider := range cfg.Spec.Providers {
		providers = append(providers, configuredProvider{Name: provider.Name})
		seen[provider.Name] = struct{}{}
	}
	for _, name := range cfg.Spec.Sandbox.Providers {
		if _, ok := seen[name]; ok {
			continue
		}
		providers = append(providers, configuredProvider{Name: name})
		seen[name] = struct{}{}
	}
	return providers
}

// runOnlineChecks checks the resolved SDK target. An empty target asks the SDK
// factory to use the active gateway registration; connection failures remain a
// warning so doctor can still report configuration problems coherently.
func runOnlineChecks(ctx context.Context, newClient openshell.Factory, target openshell.Target, providers []string) []CheckResult {
	client, err := newClient(ctx, target)
	if err != nil {
		status := "warn"
		if errors.Is(err, openshell.ErrConfig) || errors.Is(err, openshell.ErrUnauthenticated) {
			status = "fail"
		}
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  status,
			Message: fmt.Sprintf("gateway checks skipped: %v", err),
		}}
	}
	defer client.Close()

	return checkOnlineSDK(ctx, client, providers)
}

// checkOnlineSDK reads gateway health and provider registration through the
// SDK client. Health maps: healthy -> pass; ErrUnauthenticated -> fail (the
// one actionable connection failure); ErrUnavailable and any other error -> warn.
// Missing referenced providers are failures because apply cannot use them.
func checkOnlineSDK(ctx context.Context, client openshell.Client, providers []string) []CheckResult {
	h, err := client.Health(ctx)
	switch {
	case err == nil && h.Healthy:
		// fall through to provider checks below
	case errors.Is(err, openshell.ErrUnauthenticated):
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "fail",
			Message: "authentication failed — check gateway credentials",
		}}
	case errors.Is(err, openshell.ErrUnavailable):
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "gateway not reachable (provider checks skipped)",
		}}
	case err != nil:
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: fmt.Sprintf("gateway health check failed: %v (provider checks skipped)", err),
		}}
	default: // err == nil but not healthy
		return []CheckResult{{
			Group:   "gateway",
			Name:    "status",
			Status:  "warn",
			Message: "gateway reports unhealthy (provider checks skipped)",
		}}
	}

	results := []CheckResult{{
		Group:   "gateway",
		Name:    "status",
		Status:  "pass",
		Message: "connected",
	}}

	provs, err := client.Providers(ctx)
	if err != nil {
		results = append(results, CheckResult{
			Group:   "gateway",
			Name:    "providers",
			Status:  "warn",
			Message: fmt.Sprintf("could not list providers: %v", err),
		})
		return results
	}

	registered := make(map[string]bool, len(provs))
	for _, p := range provs {
		registered[p.Name] = true
	}
	for _, name := range providers {
		if registered[name] {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    name,
				Status:  "pass",
				Message: "registered",
			})
		} else {
			results = append(results, CheckResult{
				Group:   "gateway",
				Name:    name,
				Status:  "fail",
				Message: "not registered (create through platform bootstrap before apply)",
			})
		}
	}

	return results
}

func printDoctorTable(results []CheckResult) {
	for _, g := range []string{"gateway"} {
		var groupResults []CheckResult
		for _, r := range results {
			if r.Group == g {
				groupResults = append(groupResults, r)
			}
		}
		if len(groupResults) == 0 {
			continue
		}

		fmt.Println("GATEWAY")
		for _, r := range groupResults {
			icon := "  "
			switch r.Status {
			case "pass":
				icon = "OK"
			case "warn":
				icon = "!!"
			case "fail":
				icon = "XX"
			}
			fmt.Printf("  %-16s %s  %s\n", r.Name, icon, r.Message)
		}
		fmt.Println()
	}
}
