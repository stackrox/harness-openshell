package sdkclient

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// TestLiveProviderUpdatePreservesCredentials is the S1-risk gate for PR4a: it
// proves, against a real gateway, that the credential-preserving copy-through in
// UpdateProvider (which sends an EMPTY credentials map, because a real Get never
// returns raw credentials) leaves the provider's stored credentials intact
// rather than wiping them — the one server semantic no unit test can reach — and
// that the mTLS identity actually holds provider:write.
//
// It is skipped unless HARNESS_E2E_GATEWAY names a registered mTLS gateway, and
// again unless HARNESS_E2E_MANAGED_PROVIDER names a real, credentialed managed
// provider in the workspace. It mutates only that provider's Config/Labels and
// restores them on every exit path, so it never leaves drift behind. Optional
// HARNESS_E2E_WORKSPACE overrides the workspace.
//
//	HARNESS_E2E_GATEWAY=openshell HARNESS_E2E_MANAGED_PROVIDER=google-vertex-ai \
//	  go test ./internal/openshell/sdkclient/ -run LiveProviderUpdatePreservesCredentials -v
func TestLiveProviderUpdatePreservesCredentials(t *testing.T) {
	gw := os.Getenv("HARNESS_E2E_GATEWAY")
	if gw == "" {
		t.Skip("set HARNESS_E2E_GATEWAY to a registered mTLS gateway to run the provider write gate")
	}
	name := os.Getenv("HARNESS_E2E_MANAGED_PROVIDER")
	if name == "" {
		t.Skip("set HARNESS_E2E_MANAGED_PROVIDER to a real credentialed managed provider to probe provider:write")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	oc, err := New(ctx, openshell.Target{Gateway: gw, Workspace: os.Getenv("HARNESS_E2E_WORKSPACE")})
	if err != nil {
		t.Fatalf("New(%q): %v", gw, err)
	}
	// Close via t.Cleanup, not defer: t.Cleanup runs in LIFO after the test's
	// deferred calls, so a deferred Close would shut the gRPC connection before
	// the restoration cleanup registered below could use it. Registered first,
	// it runs last. (Same ordering fix as TestLiveInferenceRoleProbe.)
	t.Cleanup(func() {
		if err := oc.Close(); err != nil {
			t.Errorf("closing client: %v", err)
		}
	})
	// In-package access to the raw SDK client, so the probe can observe the
	// credential handles the firewall Provider deliberately hides.
	raw := oc.(*client)

	before, err := oc.GetProvider(ctx, name)
	if err != nil {
		t.Fatalf("GetProvider(%q): %v (is it registered in the workspace?)", name, err)
	}
	beforeRaw, err := raw.raw.Providers().Get(ctx, raw.workspace, name)
	if err != nil {
		t.Fatalf("raw Get(%q): %v", name, err)
	}
	beforeHandles := beforeRaw.Spec.CredentialHandles
	beforeExpiry := beforeRaw.Spec.CredentialExpiresAt
	if len(beforeHandles) == 0 {
		t.Logf("WARNING: provider %q reports no credential handles; the handle-survival "+
			"assertion is inconclusive. Point HARNESS_E2E_INFERENCE_PROVIDER at this "+
			"provider and run the inference probe for the definitive credential-works proof.", name)
	}

	// Register restoration BEFORE the write. UpdateProvider persists at the
	// gateway before its response returns, so even a failed write may have
	// changed state; t.Cleanup runs on every exit path (fresh context, since ctx
	// may be spent). Skipped only on a pre-write permission denial: nothing was
	// written and the identity lacks the role the restore would need.
	permissionDenied := false
	t.Cleanup(func() {
		if permissionDenied {
			return
		}
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		if _, err := oc.UpdateProvider(cctx, openshell.Provider{
			Name: name, Config: before.Config, Labels: before.Labels,
		}); err != nil {
			t.Errorf("restoring provider Config/Labels: %v", err)
		}
	})

	// Config-only mutation: carry the current Labels through unchanged (Update
	// overlays both fields) and flip one probe key in Config. This is the exact
	// production destructive path — an empty-credentials copy-through Update.
	probeConfig := copyStringMap(before.Config)
	if probeConfig == nil {
		probeConfig = map[string]string{}
	}
	probeConfig["harness.openshell.dev/e2e-probe"] = "1"
	_, setErr := oc.UpdateProvider(ctx, openshell.Provider{
		Name: name, Config: probeConfig, Labels: before.Labels,
	})
	switch {
	case setErr == nil:
		t.Logf("WRITE path OK on gateway %q: identity HAS provider:write", gw)
	case errors.Is(setErr, openshell.ErrPermission):
		permissionDenied = true
		t.Fatalf("WRITE path DENIED on gateway %q: identity LACKS provider:write "+
			"(provider reconcile-write will fail until granted): %v", gw, setErr)
	default:
		t.Fatalf("UpdateProvider returned an unexpected error: %v", setErr)
	}

	afterRaw, err := raw.raw.Providers().Get(ctx, raw.workspace, name)
	if err != nil {
		t.Fatalf("raw Get(%q) after update: %v", name, err)
	}
	if !reflect.DeepEqual(beforeHandles, afterRaw.Spec.CredentialHandles) {
		t.Fatalf("CREDENTIALS WIPED: credential handles changed after a Config-only "+
			"update.\n  before: %v\n  after:  %v\nempty-map-means-WIPE — the "+
			"copy-through Update is UNSAFE on this gateway; disable managed provider "+
			"Update until a config-only RPC or cred-resupply path exists.",
			beforeHandles, afterRaw.Spec.CredentialHandles)
	}
	if !reflect.DeepEqual(beforeExpiry, afterRaw.Spec.CredentialExpiresAt) {
		t.Fatalf("CREDENTIALS ROTATED/WIPED: credential expiry changed after a "+
			"Config-only update.\n  before: %v\n  after:  %v", beforeExpiry, afterRaw.Spec.CredentialExpiresAt)
	}
	if afterRaw.Spec.Config["harness.openshell.dev/e2e-probe"] != "1" {
		t.Errorf("probe config key not persisted: %v", afterRaw.Spec.Config)
	}
	t.Logf("PASS on gateway %q: empty-map credentials Update = LEAVE-UNTOUCHED; "+
		"copy-through provider Update is safe.", gw)

	// Stronger, definitive proof (slice S2 step 4): unchanged handles show the
	// credentials survived STRUCTURALLY, but only a real call proves they still
	// AUTHENTICATE. When this same provider also backs inference, a verify-mode
	// route write (NoVerify:false) makes the gateway call the provider endpoint
	// with its stored credentials; success after the config-only update is the
	// end-to-end guarantee. It needs a model known-good for the provider
	// (HARNESS_E2E_INFERENCE_MODEL) so a failure means "credentials broke", not
	// "unknown model" — without one this layer is skipped, not guessed.
	if os.Getenv("HARNESS_E2E_INFERENCE_PROVIDER") != name {
		return
	}
	model := os.Getenv("HARNESS_E2E_INFERENCE_MODEL")
	if model == "" {
		t.Logf("skipping downstream inference-verify proof: set HARNESS_E2E_INFERENCE_MODEL "+
			"to a model valid for %q to enable the definitive credentials-still-authenticate check", name)
		return
	}

	const verifyRoute = "inference.local" // the only route name a real gateway accepts (see inference_e2e_test.go)
	beforeRoute, beforeRouteErr := oc.GetInferenceRoute(ctx, verifyRoute)
	if beforeRouteErr != nil && !errors.Is(beforeRouteErr, openshell.ErrNotFound) {
		t.Fatalf("pre-verify GetInferenceRoute(%q): %v", verifyRoute, beforeRouteErr)
	}
	routeExisted := beforeRouteErr == nil
	// Registered last → runs first (LIFO), so the route is restored while the
	// client is still open, ahead of the provider-config restore and Close above.
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		if routeExisted {
			if _, err := oc.SetInferenceRoute(cctx, openshell.InferenceRouteConfig{
				Provider: beforeRoute.Provider, Model: beforeRoute.Model, Route: verifyRoute,
				NoVerify: true, TimeoutSecs: beforeRoute.TimeoutSecs,
			}); err != nil {
				t.Errorf("restoring inference route: %v", err)
			}
		} else if err := oc.DeleteInferenceRoute(cctx, verifyRoute); err != nil {
			t.Errorf("cleanup DeleteInferenceRoute(%q): %v", verifyRoute, err)
		}
	})
	if _, err := oc.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: name, Model: model, Route: verifyRoute, NoVerify: false,
	}); err != nil {
		t.Fatalf("DOWNSTREAM VERIFY FAILED after the config-only update: provider %q's "+
			"credentials no longer authenticate (verify route %q/%q): %v", name, verifyRoute, model, err)
	}
	t.Logf("DOWNSTREAM VERIFY OK: provider %q credentials still authenticate after the update — "+
		"copy-through Update is safe end-to-end.", name)
}
