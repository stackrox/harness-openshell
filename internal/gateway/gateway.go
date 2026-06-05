package gateway

// Gateway abstracts all OpenShell gateway operations. The CLI implementation
// shells out to the openshell binary; a future gRPC implementation will call
// the gateway API directly.
type Gateway interface {
	// CLIVersion returns the openshell CLI version string, or empty if not found.
	CLIVersion() string

	// CLIPath returns the path to the openshell CLI binary, or empty if not found.
	CLIPath() string

	// InferenceGet checks if the gateway is active and reachable.
	InferenceGet() error

	// InferenceModel returns the configured inference model name, or empty.
	InferenceModel() string

	// ActiveGateway returns the name of the currently selected gateway, or empty.
	ActiveGateway() string

	// ProviderGet checks if a provider is registered. Returns nil if it exists.
	ProviderGet(name string) error

	// ProviderList returns the names of all registered providers.
	ProviderList() ([]string, error)

	// Provider lifecycle
	ProviderCreate(name, providerType string, opts ProviderCreateOpts) error
	ProviderDelete(name string) error
	ProviderProfileImport(dir string) error
	ProviderProfileDelete(id string) error

	// Inference config
	InferenceSet(provider, model string) error
	InferenceRemove() error

	// Settings
	SettingsSet(key, value string) error

	// Sandbox lifecycle
	SandboxList() ([]string, error)
	SandboxCreate(opts SandboxCreateOpts) error
	SandboxDelete(name string) error
	SandboxConnect(name string) error
	SandboxUpload(name, localDir, remotePath string) error
	SandboxExec(name string, command ...string) error

	// Gateway management
	GatewayAdd(endpoint, name string, local bool) error
	GatewayRemove(name string) error
	GatewayList() ([]GatewayInfo, error)
	GatewaySelect(name string) error
}

type ProviderCreateOpts struct {
	Credentials []string // "KEY=VALUE" pairs
	Configs     []string // "KEY=VALUE" pairs
	FromADC     bool     // --from-gcloud-adc
}

type GatewayInfo struct {
	Name     string
	Endpoint string
	Active   bool
}

type SandboxCreateOpts struct {
	Name      string
	Image     string
	Providers []string
	TTY       bool
	Keep      bool
	UploadSrc string
	UploadDst string
	Command   []string
}
