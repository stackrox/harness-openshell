package openshell

// Target identifies what to connect to.
//
// Gateway is the OPENSHELL REGISTRATION name — the directory under
// ~/.config/openshell/gateways/<name> managed by the openshell CLI. It is NOT a
// harness gateway profile (e.g. "openshift", "local-container"); those name
// deployment recipes, not registered gateways. Never pass an agent.AgentConfig
// gateway profile here.
type Target struct {
	Gateway   string // required; openshell registration name
	Workspace string // "" defaults to "default" (defaulting owned by sdkclient)
}

// Health is the harness view of a gateway health check.
type Health struct {
	Healthy bool
	Version string
}

// Provider is the minimal harness view of a registered provider.
//
// Deliberately minimal; widened only as consumers need more fields
// (least-exposure firewall).
type Provider struct {
	Name string
	Type string
}
