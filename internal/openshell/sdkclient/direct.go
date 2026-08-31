package sdkclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

const oidcClientSecretEnv = "OPENSHELL_OIDC_CLIENT_SECRET"

// newDirect fills the one gap in the pinned OpenShell Go SDK's OIDC helper: it
// does not yet send an audience with client-credentials requests. Remove this
// bootstrap when the SDK exposes audience-aware direct gateway construction.
func newDirect(ctx context.Context, target openshell.Target) (openshell.Client, error) {
	connection := target.Direct
	if err := requireHTTPS("gateway endpoint", connection.Endpoint); err != nil {
		return nil, err
	}
	secret := os.Getenv(oidcClientSecretEnv)
	if secret == "" {
		return nil, fmt.Errorf("%w: %s is required for direct OIDC authentication", openshell.ErrConfig, oidcClientSecretEnv)
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	source, err := oidcTokenSource(ctx, connection.OIDC, secret, httpClient)
	if err != nil {
		return nil, err
	}
	auth, err := v1.RefreshableToken(source)
	if err != nil {
		return nil, fmt.Errorf("%w: configure OIDC token refresh: %v", openshell.ErrConfig, err)
	}
	raw, err := v1.NewClient(v1.Config{Address: connection.Endpoint, Auth: auth})
	if err != nil {
		return nil, fmt.Errorf("%w: dial gateway %q: %v", openshell.ErrConfig, target.Gateway, err)
	}

	c := newClient(raw, target.Workspace)
	c.gatewayName = target.Gateway
	c.gatewayEndpoint = connection.Endpoint
	return c, nil
}

func oidcTokenSource(ctx context.Context, oidc openshell.OIDCConnection, secret string, client *http.Client) (oauth2.TokenSource, error) {
	if err := requireHTTPS("OIDC issuer", oidc.Issuer); err != nil {
		return nil, err
	}
	if oidc.ClientID == "" || oidc.Audience == "" {
		return nil, fmt.Errorf("%w: OIDC client ID and audience are required", openshell.ErrConfig)
	}

	discoveryURL := strings.TrimRight(oidc.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build OIDC discovery request: %v", openshell.ErrConfig, err)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: discover OIDC issuer: %v", openshell.ErrUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: OIDC discovery returned HTTP %d", openshell.ErrUnauthenticated, response.StatusCode)
	}
	var metadata struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("%w: decode OIDC discovery metadata: %v", openshell.ErrConfig, err)
	}
	if err := requireHTTPS("OIDC token endpoint", metadata.TokenEndpoint); err != nil {
		return nil, err
	}

	tokenContext := context.WithValue(ctx, oauth2.HTTPClient, client)
	base := (&clientcredentials.Config{
		ClientID:     oidc.ClientID,
		ClientSecret: secret,
		TokenURL:     metadata.TokenEndpoint,
		EndpointParams: url.Values{
			"audience": []string{oidc.Audience},
		},
	}).TokenSource(tokenContext)
	initial, err := base.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: request OIDC client-credentials token: %v", openshell.ErrUnauthenticated, err)
	}
	return oauth2.ReuseTokenSource(initial, base), nil
}

func requireHTTPS(label, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%w: %s must be an absolute HTTPS URL", openshell.ErrConfig, label)
	}
	return nil
}
