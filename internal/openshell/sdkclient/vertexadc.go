package sdkclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	gateway "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/gateway"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// This file is the SDK-native replacement for the CLI's
// `openshell provider create --type google-vertex-ai --from-gcloud-adc`. It is
// the one credentialed provider-create path that lives below the credentials
// firewall: the gcloud refresh-token material flows straight into the gateway's
// refresh configuration and never touches the credential-free
// openshell.Provider type (invariant 26). The Create call itself carries no
// secret at all — the secret rides Refresh().Configure — so the harness-facing
// Provider vocabulary stays credential-free even here.

const (
	// vertexProviderType is the builtin provider profile id for Vertex AI. The
	// provider's Type doubles as the profile the gateway resolves.
	vertexProviderType = "google-vertex-ai"

	// vertexADCCredentialKey is the credential the gateway mints/rotates from the
	// ADC refresh token. It is the ya29.* access token the Vertex endpoints
	// authenticate with. Matches the CLI --from-gcloud-adc path.
	vertexADCCredentialKey = "GOOGLE_VERTEX_AI_TOKEN"

	// vertexProjectConfigKey and vertexRegionConfigKey are the non-secret config
	// keys the Vertex profile requires. Region (not "location") is the servable
	// dimension for Anthropic-on-Vertex models.
	vertexProjectConfigKey = "VERTEX_AI_PROJECT_ID"
	vertexRegionConfigKey  = "VERTEX_AI_REGION"

	// adcTypeAuthorizedUser is the only ADC form this path accepts. gcloud writes
	// it for user-based Application Default Credentials; service-account key files
	// (type "service_account") use a different refresh strategy and are rejected
	// here with a clear message rather than silently mis-configured.
	adcTypeAuthorizedUser = "authorized_user"
)

// ADC holds the OAuth2 refresh-token material extracted from a gcloud
// Application Default Credentials file (the authorized_user form). Only the
// three fields the gateway needs to mint Vertex AI access tokens server-side are
// kept; the raw file (which may carry other fields) is not retained.
type ADC struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

// adcFile is the subset of the gcloud ADC JSON this path reads.
type adcFile struct {
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// ReadGcloudADC reads and validates a gcloud Application Default Credentials
// file. When path is empty it resolves the conventional location
// (GOOGLE_APPLICATION_CREDENTIALS, then $CLOUDSDK_CONFIG, then
// ~/.config/gcloud/application_default_credentials.json). It accepts only the
// authorized_user form and requires all three OAuth2 fields to be present, so a
// caller never ships a half-populated credential to the gateway.
func ReadGcloudADC(path string) (ADC, error) {
	if path == "" {
		p, err := defaultADCPath()
		if err != nil {
			return ADC{}, err
		}
		path = p
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ADC{}, fmt.Errorf("%w: read gcloud ADC %q: %v", openshell.ErrConfig, path, err)
	}

	var f adcFile
	if err := json.Unmarshal(data, &f); err != nil {
		return ADC{}, fmt.Errorf("%w: parse gcloud ADC %q: %v", openshell.ErrConfig, path, err)
	}

	if f.Type != adcTypeAuthorizedUser {
		return ADC{}, fmt.Errorf("%w: gcloud ADC %q is type %q, want %q "+
			"(service-account key files are not supported by this path; run `gcloud auth application-default login`)",
			openshell.ErrInvalidArgument, path, f.Type, adcTypeAuthorizedUser)
	}

	adc := ADC{ClientID: f.ClientID, ClientSecret: f.ClientSecret, RefreshToken: f.RefreshToken}
	if adc.ClientID == "" || adc.ClientSecret == "" || adc.RefreshToken == "" {
		return ADC{}, fmt.Errorf("%w: gcloud ADC %q is missing client_id, client_secret, or refresh_token",
			openshell.ErrInvalidArgument, path)
	}
	return adc, nil
}

// DefaultADCPath resolves the conventional gcloud ADC location without reading
// it, so callers that need the ADC file for something other than the refresh
// material (e.g. reading quota_project_id) resolve the exact same path
// ReadGcloudADC("") would. See defaultADCPath for the resolution order.
func DefaultADCPath() (string, error) { return defaultADCPath() }

// defaultADCPath resolves the conventional gcloud ADC location without reading
// it. GOOGLE_APPLICATION_CREDENTIALS wins when set (gcloud and the Google
// libraries honor it first), then $CLOUDSDK_CONFIG/application_default_credentials.json,
// then the OS default under the home config dir.
func defaultADCPath() (string, error) {
	if p := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); p != "" {
		return p, nil
	}
	if dir := os.Getenv("CLOUDSDK_CONFIG"); dir != "" {
		return filepath.Join(dir, "application_default_credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: locate gcloud ADC: %v", openshell.ErrConfig, err)
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"), nil
}

// vertexADCProvider builds the SDK Provider for the Create step. It carries only
// the non-secret Type and Config — Spec.Credentials is deliberately left nil so
// the create request never transports a secret (the secret rides
// vertexADCRefreshConfig). This is the firewall property, made pure and
// unit-assertable.
func vertexADCProvider(name string, config map[string]string) *v1.Provider {
	return &v1.Provider{
		Name: name,
		Type: vertexProviderType,
		Spec: v1.ProviderSpec{Config: copyStringMap(config)},
	}
}

// vertexADCRefreshConfig builds the RefreshConfig for the Configure step: the
// gateway-owned OAuth2 refresh-token strategy fed the ADC material. client_id is
// non-secret; client_secret and refresh_token are marked secret so the gateway
// stores them under its credential storage rather than echoing them back.
func vertexADCRefreshConfig(name string, adc ADC) *v1.RefreshConfig {
	return &v1.RefreshConfig{
		Provider:      name,
		CredentialKey: vertexADCCredentialKey,
		Strategy:      v1.RefreshStrategyOAuth2RefreshToken,
		Material: map[string]string{
			"client_id":     adc.ClientID,
			"client_secret": adc.ClientSecret,
			"refresh_token": adc.RefreshToken,
		},
		SecretMaterialKeys: []string{"client_secret", "refresh_token"},
	}
}

// CreateVertexProviderFromADC registers a google-vertex-ai provider on the
// target gateway from gcloud ADC material, mirroring the CLI's
// `provider create --from-gcloud-adc`. It dials its own SDK client (this is a
// credentialed entry point, distinct from the firewall openshell.Client) and
// runs the gateway's ADC contract:
//
//  1. Create the provider with non-secret config only (no credential).
//  2. Configure gateway-owned OAuth2 refresh from the ADC refresh token.
//  3. Rotate to mint the initial access token.
//
// The gateway then holds the refresh token and re-mints ya29.* access tokens
// server-side; the harness never sees them again. Direct (OIDC) connections are
// not supported here — this create path is only exercised against CLI-managed
// gateways.
func CreateVertexProviderFromADC(ctx context.Context, t openshell.Target, name string, config map[string]string, adc ADC) error {
	raw, workspace, err := dialRaw(t)
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()
	return createVertexProviderFromADC(ctx, raw, workspace, name, config, adc)
}

// createVertexProviderFromADC is the injectable orchestration seam: it runs the
// three-step ADC contract against an already-dialed SDK client, so tests can
// drive it with the SDK fake. It validates the inputs the gateway would
// otherwise reject with an opaque error, then Creates, Configures, and Rotates.
func createVertexProviderFromADC(ctx context.Context, raw v1.ClientInterface, workspace, name string, config map[string]string, adc ADC) error {
	if name == "" {
		return fmt.Errorf("%w: provider name must not be empty", openshell.ErrInvalidArgument)
	}
	if config[vertexProjectConfigKey] == "" || config[vertexRegionConfigKey] == "" {
		return fmt.Errorf("%w: vertex config requires %s and %s",
			openshell.ErrInvalidArgument, vertexProjectConfigKey, vertexRegionConfigKey)
	}
	if adc.ClientID == "" || adc.ClientSecret == "" || adc.RefreshToken == "" {
		return fmt.Errorf("%w: ADC material is incomplete", openshell.ErrInvalidArgument)
	}

	if _, err := raw.Providers().Create(ctx, workspace, vertexADCProvider(name, config)); err != nil {
		return fmt.Errorf("create provider %q: %w", name, translate(err))
	}
	// Create persists the provider before it has a working credential. If
	// Configure or Rotate fails, roll the Create back so the provider does not
	// linger half-registered: registerADC's ProviderGet would otherwise see it
	// as existing and skip re-registration, leaving an unusable provider no
	// later run can repair.
	if _, err := raw.Providers().Refresh().Configure(ctx, workspace, vertexADCRefreshConfig(name, adc)); err != nil {
		return rollbackVertexProvider(ctx, raw, workspace, name, fmt.Errorf("configure refresh for %q: %w", name, translate(err)))
	}
	if _, err := raw.Providers().Refresh().Rotate(ctx, workspace, name, vertexADCCredentialKey); err != nil {
		return rollbackVertexProvider(ctx, raw, workspace, name, fmt.Errorf("rotate initial credential for %q: %w", name, translate(err)))
	}
	return nil
}

// rollbackVertexProvider deletes a provider whose refresh setup failed and
// returns the original cause. The delete is best-effort: its failure is
// appended but never masks the setup error the caller needs to see.
func rollbackVertexProvider(ctx context.Context, raw v1.ClientInterface, workspace, name string, cause error) error {
	if err := raw.Providers().Delete(ctx, workspace, name); err != nil {
		return fmt.Errorf("%w (rollback delete of %q also failed: %v)", cause, name, translate(err))
	}
	return cause
}

// dialRaw resolves and dials the raw SDK client for a CLI-managed gateway,
// returning the connection and the effective workspace. It mirrors New's dial
// path (LoadConfig -> planConnection -> dial) but hands back the raw client the
// credentialed ADC flow needs (Refresh() is not exposed on the firewall
// openshell.Client). Direct connections are rejected.
func dialRaw(t openshell.Target) (v1.ClientInterface, string, error) {
	if t.Direct != nil {
		return nil, "", fmt.Errorf("%w: SDK-native ADC create is not supported over a Direct connection", openshell.ErrUnsupported)
	}
	cfg, err := gateway.LoadConfig(t.Gateway)
	if err != nil {
		return nil, "", fmt.Errorf("%w: load gateway %q: %v", openshell.ErrConfig, t.Gateway, err)
	}
	plan, err := planConnection(cfg)
	if err != nil {
		return nil, "", err
	}
	raw, err := dial(plan)
	if err != nil {
		return nil, "", err
	}
	workspace := t.Workspace
	if workspace == "" {
		workspace = defaultWorkspace
	}
	return raw, workspace, nil
}
