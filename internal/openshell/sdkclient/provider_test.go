package sdkclient

import (
	"testing"
	"time"

	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
)

// TestFromSDKProviderMapsConfigAndLabels pins the S1 read-widening: the harness
// Provider carries the SDK provider's non-secret Config and Labels, as fresh
// copies, and never the secret Spec fields.
func TestFromSDKProviderMapsConfigAndLabels(t *testing.T) {
	sdkConfig := map[string]string{"VERTEX_AI_REGION": "global"}
	sdkLabels := map[string]string{"harness.openshell.dev/managed-by": "harness"}
	p := &types.Provider{
		Name:            "google-vertex-ai",
		Type:            "google-vertex-ai",
		ResourceVersion: 7,
		Labels:          sdkLabels,
		Spec: types.ProviderSpec{
			Config:      sdkConfig,
			Credentials: map[string]string{"API_KEY": "secret"}, // must NOT cross
			CredentialHandles: map[string]types.CredentialHandle{
				"API_KEY": {Driver: "vault", Handle: "h1"},
			},
			CredentialExpiresAt: map[string]time.Time{"API_KEY": {}},
		},
	}

	got := fromSDKProvider(p)

	if got.Name != "google-vertex-ai" || got.Type != "google-vertex-ai" {
		t.Fatalf("name/type not mapped: %+v", got)
	}
	if got.Config["VERTEX_AI_REGION"] != "global" {
		t.Errorf("Config not mapped: %v", got.Config)
	}
	if got.Labels["harness.openshell.dev/managed-by"] != "harness" {
		t.Errorf("Labels not mapped: %v", got.Labels)
	}

	// Fresh copies: mutating the source must not affect the harness view.
	sdkConfig["VERTEX_AI_REGION"] = "us-east1"
	sdkLabels["harness.openshell.dev/managed-by"] = "someone-else"
	if got.Config["VERTEX_AI_REGION"] != "global" {
		t.Errorf("Config aliases the SDK map: %v", got.Config)
	}
	if got.Labels["harness.openshell.dev/managed-by"] != "harness" {
		t.Errorf("Labels aliases the SDK map: %v", got.Labels)
	}
}

// TestFromSDKProviderEmptyMapsAreNil keeps the harness view tidy: absent
// Config/Labels map to nil, not empty non-nil maps.
func TestFromSDKProviderEmptyMapsAreNil(t *testing.T) {
	got := fromSDKProvider(&types.Provider{Name: "p", Type: "openai"})
	if got.Config != nil {
		t.Errorf("expected nil Config, got %v", got.Config)
	}
	if got.Labels != nil {
		t.Errorf("expected nil Labels, got %v", got.Labels)
	}
}
