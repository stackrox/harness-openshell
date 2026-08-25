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

// InferenceRoute is the harness view of an inference route read from a gateway.
//
// Deliberately minimal (least-exposure firewall): only the fields the harness
// diffs or reports. Version is server-assigned and increments on every write.
type InferenceRoute struct {
	Provider    string
	Model       string
	Route       string
	TimeoutSecs uint64
	Version     uint64
}

// InferenceRouteConfig is a desired inference route to write.
//
// Provider and Model are required. Route "" targets the gateway default route.
// NoVerify skips the gateway's synchronous endpoint validation. TimeoutSecs 0
// lets the gateway apply its default.
type InferenceRouteConfig struct {
	Provider    string
	Model       string
	Route       string
	NoVerify    bool
	TimeoutSecs uint64
}
