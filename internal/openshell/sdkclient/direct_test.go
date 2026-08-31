package sdkclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestOIDCTokenSourceDiscoversAndSendsAudience(t *testing.T) {
	const (
		clientID = "ci-user"
		secret   = "not-rendered"
		audience = "openshell-gateway"
	)
	var tokenRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/issuer/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token_endpoint": server.URL + "/token"})
		case "/token":
			tokenRequests.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token request: %v", err)
			}
			gotID, gotSecret, ok := r.BasicAuth()
			if !ok {
				gotID, gotSecret = r.Form.Get("client_id"), r.Form.Get("client_secret")
			}
			if gotID != clientID || gotSecret != secret {
				t.Fatalf("unexpected client authentication: id=%q", gotID)
			}
			if got := r.Form.Get("audience"); got != audience {
				t.Fatalf("audience=%q, want %q", got, audience)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "gateway-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source, err := oidcTokenSource(context.Background(), openshell.OIDCConnection{
		Issuer: server.URL + "/issuer", ClientID: clientID, Audience: audience,
	}, secret, server.Client())
	if err != nil {
		t.Fatalf("oidcTokenSource: %v", err)
	}
	token, err := source.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token.AccessToken != "gateway-token" {
		t.Fatalf("access token=%q", token.AccessToken)
	}
	if tokenRequests.Load() != 1 {
		t.Fatalf("token requests=%d, want eager request reused", tokenRequests.Load())
	}
}

func TestOIDCTokenSourceRejectsInsecureIssuer(t *testing.T) {
	_, err := oidcTokenSource(context.Background(), openshell.OIDCConnection{
		Issuer: "http://issuer.example.com", ClientID: "ci", Audience: "gateway",
	}, "secret", &http.Client{Timeout: time.Second})
	if err == nil {
		t.Fatal("expected insecure issuer to be rejected")
	}
}

func TestRequireHTTPS(t *testing.T) {
	for _, raw := range []string{"", "gateway.example.com", "http://gateway.example.com", "https://user@gateway.example.com"} {
		if err := requireHTTPS("gateway endpoint", raw); err == nil {
			t.Errorf("requireHTTPS(%q) succeeded", raw)
		}
	}
	if err := requireHTTPS("gateway endpoint", "https://gateway.example.com"); err != nil {
		t.Fatalf("valid endpoint rejected: %v", err)
	}
}
