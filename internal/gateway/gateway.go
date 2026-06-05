package gateway

// Focused sub-interfaces aligned with OpenShell proto service domains.
// Each consumer imports only the interface it needs, simplifying mocks.

// ProviderManager handles provider CRUD and profile operations.
type ProviderManager interface {
	ProviderGet(name string) error
	ProviderCreate(name, providerType string, opts ProviderCreateOpts) error
	ProviderDelete(name string) error
	ProviderList() ([]string, error)
	ProviderProfileImport(dir string) error
	ProviderProfileDelete(id string) error
}

// SandboxManager handles sandbox lifecycle operations.
type SandboxManager interface {
	SandboxList() ([]string, error)
	SandboxCreate(opts SandboxCreateOpts) error
	SandboxDelete(name string) error
	SandboxConnect(name string) error
	SandboxUpload(name, localDir, remotePath string) error
	SandboxExec(name string, command ...string) error
}

// InferenceConfig handles inference routing configuration.
type InferenceConfig interface {
	InferenceGet() error
	InferenceModel() string
	InferenceSet(provider, model string) error
	InferenceRemove() error
}

// GatewayAdmin handles gateway management, CLI detection, and settings.
type GatewayAdmin interface {
	CLIVersion() string
	CLIPath() string
	ActiveGateway() string
	GatewayAdd(endpoint, name string, local bool) error
	GatewayRemove(name string) error
	GatewayList() ([]GatewayInfo, error)
	GatewaySelect(name string) error
	SettingsSet(key, value string) error
}

// Gateway composes all sub-interfaces. Use this when a function needs
// multiple domains. Prefer the narrower interfaces when possible.
type Gateway interface {
	ProviderManager
	SandboxManager
	InferenceConfig
	GatewayAdmin
}

type ProviderCreateOpts struct {
	Credentials []string
	Configs     []string
	FromADC     bool
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
