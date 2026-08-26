package cmd

import (
	"testing"

	"github.com/stackrox/harness-openshell/internal/agent"
)

func noEnv(string) string { return "" }

func TestDesiredFromAgent_InferenceFromVertexProvider(t *testing.T) {
	agentCfg := &agent.AgentConfig{
		Providers: []agent.ProviderRef{
			{Profile: "github"},
			{Profile: "google-vertex-ai"},
			{Profile: "atlassian"},
		},
	}

	_, inf := desiredFromAgent(agentCfg, noEnv)

	if inf.Provider != "google-vertex-ai" {
		t.Errorf("inference provider = %q, want google-vertex-ai", inf.Provider)
	}
	if inf.Model != "claude-sonnet-4-6" {
		t.Errorf("inference model = %q, want default claude-sonnet-4-6", inf.Model)
	}
	// Verify unset → verify-by-default (the S5 behavior change).
	if inf.Verify != nil {
		t.Errorf("inference Verify = %v, want nil (verify-by-default)", *inf.Verify)
	}
	if !inf.VerifyEnabled() {
		t.Error("VerifyEnabled() = false, want true for unset Verify")
	}
}

func TestDesiredFromAgent_ModelFromEnv(t *testing.T) {
	agentCfg := &agent.AgentConfig{
		Providers: []agent.ProviderRef{{Profile: "google-vertex-ai"}},
	}
	getenv := func(k string) string {
		if k == "OPENSHELL_MODEL" {
			return "claude-opus-4-8"
		}
		return ""
	}

	_, inf := desiredFromAgent(agentCfg, getenv)

	if inf.Model != "claude-opus-4-8" {
		t.Errorf("inference model = %q, want claude-opus-4-8 from env", inf.Model)
	}
}

func TestDesiredFromAgent_NoInferenceProvider(t *testing.T) {
	agentCfg := &agent.AgentConfig{
		Providers: []agent.ProviderRef{
			{Profile: "github"},
			{Profile: "atlassian"},
		},
	}

	_, inf := desiredFromAgent(agentCfg, noEnv)

	if inf.Provider != "" || inf.Model != "" {
		t.Errorf("inference = %+v, want empty (no inference provider configured)", inf)
	}
}

func TestDesiredFromAgent_ProviderClassification(t *testing.T) {
	agentCfg := &agent.AgentConfig{
		Providers: []agent.ProviderRef{
			{Profile: "github"},
			{Profile: "google-vertex-ai"},
			{Profile: "google-workspace"},
			{Profile: "atlassian"},
		},
	}

	providers, _ := desiredFromAgent(agentCfg, noEnv)

	if len(providers) != 4 {
		t.Fatalf("got %d providers, want 4", len(providers))
	}
	want := map[string]string{
		"github":           "referenced",
		"google-vertex-ai": "managed",
		"google-workspace": "managed",
		"atlassian":        "referenced",
	}
	for _, p := range providers {
		if p.Management != want[p.Name] {
			t.Errorf("%s: Management = %q, want %q", p.Name, p.Management, want[p.Name])
		}
	}
}
