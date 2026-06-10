package profile

import (
	"os"
)

// ProviderChecker checks if a provider is registered.
type ProviderChecker interface {
	ProviderGet(name string) error
}

type Config struct {
	Name      string            `toml:"name"`
	From      string            `toml:"from"`
	Command   string            `toml:"command"`
	Startup   string            `toml:"startup"`
	Keep      *bool             `toml:"keep"`
	Providers []string          `toml:"providers"`
	Env       map[string]string `toml:"env"`
}

func (c *Config) KeepSandbox() bool {
	if c.Keep == nil {
		return true
	}
	return *c.Keep
}


// ValidateProviders checks which profile providers are registered on the
// gateway. Returns the list of registered providers and a list of missing ones.
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

// StageHarnessDir creates the harness directory.
func StageHarnessDir(harnessDir string) error {
	return os.MkdirAll(harnessDir, 0o755)
}
