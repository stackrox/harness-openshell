package gateway

// Gateway abstracts the openshell CLI operations the harness still shells out
// for: credentialed provider bootstrap and sandbox create/delete for the rich
// (upload/policy/TTY/local-image) cases. Everything else — read, reconcile,
// inference, health, and get/describe/delete — is on the SDK
// (internal/openshell/sdkclient); no CLI stdout/table parsing remains here.
type Gateway interface {
	// Providers. Reference (--from-existing) create, gws OAuth refresh, and
	// profile import stay on the CLI bridge; the gcloud-ADC create is SDK-native
	// (sdkclient.CreateVertexProviderFromADC — the refresh token rides the
	// gateway's refresh config, never the firewall Provider type). Once a
	// provider exists the SDK reconcile owns verify/update/adoption.
	ProviderGet(name string) error
	ProviderCreate(name, providerType string, opts ProviderCreateOpts) error
	ProviderProfileImport(dir string) error
	ProviderRefreshConfigure(name string, opts ProviderRefreshOpts) error
	ProviderRefreshRotate(name, credentialKey string) error

	// Sandboxes. Create backs the legacy front-end and canonical local-image
	// builds; Delete backs the CLI create retry cleanup. Reachability,
	// get/describe/delete are on the SDK.
	SandboxCreate(opts SandboxCreateOpts) error
	SandboxDelete(name string) error
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
	FromExisting bool
}

type ProviderRefreshOpts struct {
	CredentialKey    string
	Strategy         string
	Material         []string // KEY=VALUE pairs
	SecretMaterialKeys []string // keys within Material that are secret
}

type Upload struct {
	Src string
	Dst string
}

type SandboxCreateOpts struct {
	Name            string
	From            string
	Providers       []string
	TTY             bool
	Keep            bool
	Uploads         []Upload
	Command         []string
	Env             map[string]string
	Policy          string            // --policy <path>
	Gateway         string            // --gateway <name>
	Workspace       string            // --workspace <name>
	Labels          map[string]string // --label k=v
	NoAutoProviders bool              // --no-auto-providers when true
}
