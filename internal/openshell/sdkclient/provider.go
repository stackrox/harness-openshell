package sdkclient

import (
	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// fromSDKProvider maps the SDK provider view to the minimal harness Provider.
// Deliberately narrow (least-exposure firewall); widen only when a consumer
// genuinely needs more fields, changing this and openshell.Provider together.
func fromSDKProvider(p *v1.Provider) openshell.Provider {
	return openshell.Provider{Name: p.Name, Type: p.Type}
}
