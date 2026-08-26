package sdkclient

import (
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
