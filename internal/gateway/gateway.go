package gateway

// Gateway abstracts all openshell CLI operations.
type Gateway interface {
	// Providers
	ProviderGet(name string) error
	ProviderCreate(name, providerType string, opts ProviderCreateOpts) error
	ProviderDelete(name string) error
	ProviderList() ([]string, error)
	ProviderProfileImport(dir string) error
	ProviderProfileDelete(id string) error
	ProviderRefreshConfigure(name string, opts ProviderRefreshOpts) error
	ProviderRefreshRotate(name, credentialKey string) error

	// Sandboxes
	SandboxList() ([]string, error)
	SandboxStatus() ([]SandboxInfo, error)
	SandboxCreate(opts SandboxCreateOpts) error
	SandboxDelete(name string) error
	SandboxConnect(name string) error
	SandboxLogs(name string, follow bool) error
	SandboxStop(name string) error
	SandboxStart(name string) error

	// Inference
	InferenceGet() error
	InferenceModel() string
	InferenceSet(provider, model string) error
	InferenceRemove() error

	// Gateway management
	CLIVersion() string
	CLIPath() string
	ActiveGateway() string
	GatewayAdd(endpoint, name string, local, insecure bool) error
	GatewayRemove(name string) error
	GatewayList() ([]GatewayInfo, error)
	GatewaySelect(name string) error
	SettingsSet(key, value string) error
}

// ProviderChecker is the subset of Gateway needed to check provider registration.
type ProviderChecker interface {
	ProviderGet(name string) error
}

// ValidateProviders checks which providers are registered on the gateway.
// Returns the list of registered providers and a list of missing ones.
func ValidateProviders(providers []string, gw ProviderChecker) (registered, missing []string) {
	for _, name := range providers {
		if gw.ProviderGet(name) == nil {
			registered = append(registered, name)
		} else {
			missing = append(missing, name)
		}
	}
	return
}

type ProviderCreateOpts struct {
	Credentials  []string
	Configs      []string
	FromADC      bool
	FromExisting bool
}

type ProviderRefreshOpts struct {
	CredentialKey    string
	Strategy         string
	Material         []string // KEY=VALUE pairs
	SecretMaterialKeys []string // keys within Material that are secret
}

type GatewayInfo struct {
	Name     string
	Endpoint string
	Active   bool
}

type SandboxInfo struct {
	Name  string
	Phase string
}

type SandboxCreateOpts struct {
	Name      string
	From      string
	Providers []string
	TTY       bool
	Keep      bool
	UploadSrc string
	UploadDst string
	Command   []string
	Env       map[string]string
}
