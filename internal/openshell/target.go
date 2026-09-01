package openshell

// Environment variables consulted by ResolveTarget, in the repo's standard
// flag-resolution order (AGENTS.md: explicit flag > OPENSHELL_* env var >
// default). Exported so cmd help text and tests can name them without
// re-declaring the strings.
const (
	EnvGateway   = "OPENSHELL_GATEWAY"
	EnvWorkspace = "OPENSHELL_WORKSPACE"
)

// ResolveTarget builds a Target from explicit flag values, environment variables,
// and config values, applying flag > env > config > empty for each field
// independently.
//
// It does NOT default the workspace: an unset workspace stays "" and sdkclient
// maps "" -> "default" at construction (the single owner of that default). An
// empty Gateway is likewise left empty; sdkclient interprets it as the active
// CLI-compatible gateway registration when a caller chooses to connect.
//
// Resolution is pure: getenv is injected (production passes os.Getenv, tests
// pass a map closure) so this package imports neither os nor any CLI framework.
func ResolveTarget(flagGateway, flagWorkspace, cfgGateway, cfgWorkspace string, getenv func(string) string) Target {
	return Target{
		Gateway:   resolveField(flagGateway, EnvGateway, cfgGateway, getenv),
		Workspace: resolveField(flagWorkspace, EnvWorkspace, cfgWorkspace, getenv),
	}
}

// resolveField applies flag > env > config > empty for one field. An empty flag
// value is treated as unset so env and config can take effect in precedence.
func resolveField(flagVal, envKey, cfgVal string, getenv func(string) string) string {
	if flagVal != "" {
		return flagVal
	}
	envVal := getenv(envKey)
	if envVal != "" {
		return envVal
	}
	return cfgVal
}
