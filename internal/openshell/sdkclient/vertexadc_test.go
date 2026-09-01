package sdkclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func writeADC(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "application_default_credentials.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadGcloudADC_Valid(t *testing.T) {
	path := writeADC(t, `{
		"type": "authorized_user",
		"client_id": "cid.apps.googleusercontent.com",
		"client_secret": "secret-xyz",
		"refresh_token": "1//refresh"
	}`)
	adc, err := ReadGcloudADC(path)
	if err != nil {
		t.Fatalf("ReadGcloudADC: %v", err)
	}
	want := ADC{ClientID: "cid.apps.googleusercontent.com", ClientSecret: "secret-xyz", RefreshToken: "1//refresh"}
	if adc != want {
		t.Errorf("ADC = %+v, want %+v", adc, want)
	}
}

func TestReadGcloudADC_RejectsServiceAccount(t *testing.T) {
	path := writeADC(t, `{"type": "service_account", "client_email": "x@y.iam.gserviceaccount.com"}`)
	_, err := ReadGcloudADC(path)
	if !errors.Is(err, openshell.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument", err)
	}
}

func TestReadGcloudADC_MissingFields(t *testing.T) {
	path := writeADC(t, `{"type": "authorized_user", "client_id": "cid", "refresh_token": "1//r"}`)
	_, err := ReadGcloudADC(path)
	if !errors.Is(err, openshell.ErrInvalidArgument) {
		t.Errorf("err = %v, want ErrInvalidArgument (missing client_secret)", err)
	}
}

func TestReadGcloudADC_MissingFile(t *testing.T) {
	_, err := ReadGcloudADC(filepath.Join(t.TempDir(), "nope.json"))
	if !errors.Is(err, openshell.ErrConfig) {
		t.Errorf("err = %v, want ErrConfig", err)
	}
}

func TestReadGcloudADC_DefaultPathViaEnv(t *testing.T) {
	path := writeADC(t, `{
		"type": "authorized_user", "client_id": "c", "client_secret": "s", "refresh_token": "r"
	}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
	adc, err := ReadGcloudADC("")
	if err != nil {
		t.Fatalf("ReadGcloudADC(\"\"): %v", err)
	}
	if adc.RefreshToken != "r" {
		t.Errorf("RefreshToken = %q, want r", adc.RefreshToken)
	}
}

// vertexADCProvider must never carry a credential: the Create request is the one
// step that, if it leaked a secret, would breach the firewall. This pins that
// Spec.Credentials stays nil while Type and Config are set.
func TestVertexADCProvider_NoCredential(t *testing.T) {
	p := vertexADCProvider("vertex-sdk", map[string]string{
		vertexProjectConfigKey: "proj",
		vertexRegionConfigKey:  "us-east5",
	})
	if p.Type != vertexProviderType {
		t.Errorf("Type = %q, want %q", p.Type, vertexProviderType)
	}
	if p.Spec.Credentials != nil {
		t.Errorf("Spec.Credentials = %v, want nil (no secret in Create)", p.Spec.Credentials)
	}
	if p.Spec.Config[vertexProjectConfigKey] != "proj" || p.Spec.Config[vertexRegionConfigKey] != "us-east5" {
		t.Errorf("Config = %v, want project+region", p.Spec.Config)
	}
}

func TestVertexADCRefreshConfig(t *testing.T) {
	rc := vertexADCRefreshConfig("vertex-sdk", ADC{ClientID: "c", ClientSecret: "s", RefreshToken: "r"})
	if rc.Strategy != v1.RefreshStrategyOAuth2RefreshToken {
		t.Errorf("Strategy = %q, want OAuth2RefreshToken", rc.Strategy)
	}
	if rc.CredentialKey != vertexADCCredentialKey {
		t.Errorf("CredentialKey = %q, want %q", rc.CredentialKey, vertexADCCredentialKey)
	}
	wantMaterial := map[string]string{"client_id": "c", "client_secret": "s", "refresh_token": "r"}
	if !reflect.DeepEqual(rc.Material, wantMaterial) {
		t.Errorf("Material = %v, want %v", rc.Material, wantMaterial)
	}
	// client_secret and refresh_token are secret; client_id is not.
	wantSecret := []string{"client_secret", "refresh_token"}
	if !reflect.DeepEqual(rc.SecretMaterialKeys, wantSecret) {
		t.Errorf("SecretMaterialKeys = %v, want %v", rc.SecretMaterialKeys, wantSecret)
	}
}

func validConfig() map[string]string {
	return map[string]string{vertexProjectConfigKey: "proj", vertexRegionConfigKey: "us-east5"}
}

func validADC() ADC { return ADC{ClientID: "c", ClientSecret: "s", RefreshToken: "r"} }

func TestCreateVertexProviderFromADC_Validation(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	cases := []struct {
		name   string
		pname  string
		config map[string]string
		adc    ADC
	}{
		{"empty name", "", validConfig(), validADC()},
		{"missing region", "vertex-sdk", map[string]string{vertexProjectConfigKey: "proj"}, validADC()},
		{"incomplete adc", "vertex-sdk", validConfig(), ADC{ClientID: "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := createVertexProviderFromADC(ctx, fc, "default", tc.pname, tc.config, tc.adc)
			if !errors.Is(err, openshell.ErrInvalidArgument) {
				t.Errorf("err = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

// The SDK fake persists Create but returns Unimplemented for Refresh().Configure
// (refresh needs a real server). This test rides that: it proves the flow (a)
// creates the provider with the right non-secret Type/Config and NO credential,
// and (b) proceeds past Create into Configure — surfaced as the fake's
// Unimplemented, mapped to ErrUnsupported. The end-to-end Configure/Rotate proof
// lives in the live gate below.
func TestCreateVertexProviderFromADC_CreatesThenConfigures(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()

	err := createVertexProviderFromADC(ctx, fc, "default", "vertex-sdk", validConfig(), validADC())
	if !errors.Is(err, openshell.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported (fake Configure is Unimplemented, proving Create succeeded and Configure ran)", err)
	}

	got, err := fc.Providers().Get(ctx, "default", "vertex-sdk")
	if err != nil {
		t.Fatalf("provider was not created: %v", err)
	}
	if got.Type != vertexProviderType {
		t.Errorf("stored Type = %q, want %q", got.Type, vertexProviderType)
	}
	if len(got.Spec.Credentials) != 0 {
		t.Errorf("stored Credentials = %v, want empty (Create must not carry a secret)", got.Spec.Credentials)
	}
	if got.Spec.Config[vertexProjectConfigKey] != "proj" {
		t.Errorf("stored Config = %v, want project set", got.Spec.Config)
	}
}

// TestLiveVertexProviderFromADC is the end-to-end gate: it runs the full
// SDK-native ADC flow (Create + Configure + Rotate) against a real gateway and
// proves the gateway minted the initial access token — the CLI-parity proof no
// fake can give (the fake's Refresh() is Unimplemented).
//
// Skipped unless HARNESS_E2E_GATEWAY names a registered gateway. It creates a
// distinctly named provider (default: vertex-sdk) so it never disturbs an
// existing google-vertex-ai inference route, and deletes it on every exit path.
// Config comes from HARNESS_E2E_VERTEX_PROJECT / HARNESS_E2E_VERTEX_REGION; ADC
// from the conventional gcloud location (or GOOGLE_APPLICATION_CREDENTIALS).
//
//	HARNESS_E2E_GATEWAY=openshell \
//	HARNESS_E2E_VERTEX_PROJECT=itpc-ca-b7242ff092 HARNESS_E2E_VERTEX_REGION=us-east5 \
//	  go test ./internal/openshell/sdkclient/ -run LiveVertexProviderFromADC -v
func TestLiveVertexProviderFromADC(t *testing.T) {
	gw := os.Getenv("HARNESS_E2E_GATEWAY")
	if gw == "" {
		t.Skip("set HARNESS_E2E_GATEWAY to run the SDK-native ADC create gate")
	}
	project := os.Getenv("HARNESS_E2E_VERTEX_PROJECT")
	region := os.Getenv("HARNESS_E2E_VERTEX_REGION")
	if project == "" || region == "" {
		t.Skip("set HARNESS_E2E_VERTEX_PROJECT and HARNESS_E2E_VERTEX_REGION to run the SDK-native ADC create gate")
	}
	name := os.Getenv("HARNESS_E2E_VERTEX_PROVIDER")
	if name == "" {
		name = "vertex-sdk"
	}

	adc, err := ReadGcloudADC(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err != nil {
		t.Fatalf("ReadGcloudADC: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	target := openshell.Target{Gateway: gw, Workspace: os.Getenv("HARNESS_E2E_WORKSPACE")}

	// Clean up the probe provider on every exit path, with a fresh context (ctx
	// may be spent). Registered before the create so a partial create is still
	// swept.
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		oc, derr := New(cctx, target)
		if derr != nil {
			t.Logf("cleanup: New: %v (provider %q may need manual deletion)", derr, name)
			return
		}
		defer func() { _ = oc.Close() }()
		if derr := oc.DeleteProvider(cctx, name); derr != nil && !errors.Is(derr, openshell.ErrNotFound) {
			t.Logf("cleanup: DeleteProvider(%q): %v", name, derr)
		}
	})

	config := map[string]string{vertexProjectConfigKey: project, vertexRegionConfigKey: region}
	if err := CreateVertexProviderFromADC(ctx, target, name, config, adc); err != nil {
		t.Fatalf("CreateVertexProviderFromADC: %v", err)
	}

	// Prove it landed and the gateway holds a live refresh: the credential
	// handle exists for GOOGLE_VERTEX_AI_TOKEN.
	oc, err := New(ctx, target)
	if err != nil {
		t.Fatalf("New (verify): %v", err)
	}
	defer func() { _ = oc.Close() }()
	raw := oc.(*client)
	statuses, err := raw.raw.Providers().Refresh().GetStatus(ctx, raw.workspace, name, vertexADCCredentialKey)
	if err != nil {
		t.Fatalf("GetStatus after create: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatalf("no refresh status for %q/%s — gateway did not configure refresh", name, vertexADCCredentialKey)
	}
	t.Logf("refresh status for %q/%s: strategy=%s status=%s expiresAt=%s",
		name, vertexADCCredentialKey, statuses[0].Strategy, statuses[0].Status, statuses[0].ExpiresAt)
}
