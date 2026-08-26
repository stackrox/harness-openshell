package cmd

import (
	"github.com/stackrox/harness-openshell/internal/agent"
	"github.com/stackrox/harness-openshell/internal/config"
)

// desiredFromAgent bridges the legacy agent-config world (agent.AgentConfig,
// agent.ProviderRef) into the reconcile world (config.Provider, config.Inference).
//
// It is the single seam between the two config models. Today apply is driven by
// agent.AgentConfig while the SDK reconcile path (plan/reconcile) speaks
// config.Harness; until apply is migrated to author config.Harness directly this
// function is where the two meet. When that migration lands, this function — and
// only this function — is deleted.
//
// It is a classifier, not per-provider credential logic: it maps profile names
// to desired resources and derives the inference route from whichever configured
// provider serves inference. It never materializes secrets and never contacts a
// gateway. getenv is injected (production passes os.Getenv) so the OPENSHELL_MODEL
// default is testable.
func desiredFromAgent(agentCfg *agent.AgentConfig, getenv func(string) string) ([]config.Provider, config.Inference) {
	model := getenv("OPENSHELL_MODEL")
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	var providers []config.Provider
	var inference config.Inference
	for _, p := range agentCfg.Providers {
		providers = append(providers, config.Provider{
			Name:       p.Profile,
			Management: managementFor(p.Profile),
		})
		// The inference route points at whichever provider serves inference.
		// Verify is left unset so config.Inference.VerifyEnabled defaults to
		// true — verify-by-default. There is deliberately no agent-config field
		// to opt out yet; the escape hatch (inference.verify: false) lives in the
		// config.Harness world consumed by `harness plan`/reconcile.
		if inferenceProviders[p.Profile] {
			inference = config.Inference{
				Provider: p.Profile,
				Model:    model,
			}
		}
	}
	return providers, inference
}

// managementFor classifies a provider profile as "managed" (the harness owns its
// lifecycle — credentials and refresh flow through the gateway) or "referenced"
// (the harness only points at an existing registration). This mirrors the legacy
// registration split in registerProviders: ADC/OAuth-refresh providers are
// managed; the rest are referenced. It is provisional — the authoritative
// classification arrives with the provider reconcile (S6).
func managementFor(profile string) string {
	switch profile {
	case "google-vertex-ai", "google-workspace":
		return "managed"
	default:
		return "referenced"
	}
}
