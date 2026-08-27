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

// Provider is the harness view of a registered provider.
//
// Deliberately narrow (least-exposure firewall): it carries only the non-secret
// fields the harness diffs, reports, or writes. It has NO Credentials field —
// credentials are write-only and never returned by the SDK's Get, so keeping
// them off this type makes credential-clobber-by-reconcile impossible by
// construction (a reconcile can neither read nor author a secret). It has NO
// ResourceVersion field either: the OCC token is an SDK detail owned entirely by
// sdkclient.UpdateProvider's copy-through, never surfaced to callers.
type Provider struct {
	Name   string
	Type   string
	Config map[string]string // non-secret managed configuration
	Labels map[string]string // ownership + metadata (see plan.OwnerLabelKey)
}

// Sandbox is the harness view of a sandbox for the read UX (get/describe).
//
// Deliberately narrow (least-exposure firewall): only the fields the read
// commands render. Phase is the SDK SandboxPhase carried through as a string
// (Provisioning|Ready|Error|Deleting|Unknown|Stopping). Widen only when a
// consumer genuinely needs more, changing this and fromSDKSandbox together.
type Sandbox struct {
	Name  string
	Phase string
}

// GatewayInfo is the harness view of the active gateway.
//
// Name and Endpoint are connection facts captured at construction (New, from the
// CLI-managed gateway config); the injection path (NewFromClient) leaves them
// empty, so Endpoint in particular is production-only and untested via the SDK
// fake. Status and Version come from the gateway's health RPC. Status is the SDK
// ServiceStatus as a string (Healthy|Degraded|Unhealthy|Unknown).
type GatewayInfo struct {
	Name     string
	Endpoint string
	Status   string
	Version  string
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
