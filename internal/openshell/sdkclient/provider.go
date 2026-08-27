package sdkclient

import (
	"context"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKProvider maps the SDK provider view to the harness Provider. It copies
// only the non-secret Config and Labels (as fresh maps, never aliasing the SDK
// object); Spec.Credentials, CredentialHandles, and ResourceVersion are
// deliberately dropped at this boundary (least-exposure firewall — see the
// openshell.Provider doc). Widen only when a consumer genuinely needs more
// fields, changing this and openshell.Provider together.
func fromSDKProvider(p *v1.Provider) openshell.Provider {
	return openshell.Provider{
		Name:   p.Name,
		Type:   p.Type,
		Config: copyStringMap(p.Spec.Config),
		Labels: copyStringMap(p.Labels),
	}
}

// copyStringMap returns a fresh copy of m, or nil when m is empty, so the
// harness view never aliases the SDK object's maps.
func copyStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// GetProvider reads the named provider in the bound workspace.
func (c *client) GetProvider(ctx context.Context, name string) (openshell.Provider, error) {
	p, err := c.raw.Providers().Get(ctx, c.workspace, name)
	if err != nil {
		return openshell.Provider{}, translate(err)
	}
	return fromSDKProvider(p), nil
}

// DeleteProvider removes the named provider in the bound workspace.
func (c *client) DeleteProvider(ctx context.Context, name string) error {
	return translate(c.raw.Providers().Delete(ctx, c.workspace, name))
}

// UpdateProvider writes the desired non-secret Config/Labels of an existing
// provider while preserving everything else the gateway holds — this is the
// single credential-preserving-update site (spec §8.5).
//
// It re-Gets the provider's current server object and overlays the non-secret
// managed fields (Config, Labels, and Type) onto it, then Updates. The
// credential-bearing spec fields
// (Credentials, CredentialHandles, CredentialExpiresAt, ProfileWorkspace) and
// the ResourceVersion are carried through from that Get verbatim; the harness
// never authors them. Because the harness openshell.Provider has no credentials
// field, this function is the ONLY place a full SDK provider object is
// assembled for a write, and it cannot introduce or drop a secret.
//
// The irreducible caveat lives here: a real gateway's Get never returns raw
// Credentials (they are write-only), so the object sent to Update carries an
// empty Credentials map. Whether the gateway reads that as "leave" or "wipe" is
// a server semantic no unit test can reach — it is proven by the gated
// TestLiveProviderUpdatePreservesCredentials. Reconcile bounds the risk by
// issuing this only on a real Config/Label delta.
func (c *client) UpdateProvider(ctx context.Context, p openshell.Provider) (openshell.Provider, error) {
	cur, err := c.raw.Providers().Get(ctx, c.workspace, p.Name)
	if err != nil {
		return openshell.Provider{}, translate(err)
	}
	// Overlay only the non-secret managed fields onto the server's own object;
	// everything else (creds, handles, expiry, profile workspace, RV) is left
	// exactly as Get returned it.
	cur.Spec.Config = copyStringMap(p.Config)
	cur.Labels = copyStringMap(p.Labels)
	if p.Type != "" {
		// Type is a non-secret managed field. ProviderAction returns Update on a
		// type delta (plan.ProviderAction), so this is the write that actually
		// applies it — overlaid only when declared, so an unset desired Type never
		// wipes the stored one.
		cur.Type = p.Type
	}
	updated, err := c.raw.Providers().Update(ctx, c.workspace, cur)
	if err != nil {
		return openshell.Provider{}, translate(err)
	}
	return fromSDKProvider(updated), nil
}
