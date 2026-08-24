package openshell

// Environment variables consulted by ResolveTarget, in the repo's standard
// flag-resolution order (AGENTS.md: explicit flag > OPENSHELL_* env var >
// default). Exported so cmd help text and tests can name them without
// re-declaring the strings.
const (
	EnvGateway   = "OPENSHELL_GATEWAY"
	EnvWorkspace = "OPENSHELL_WORKSPACE"
)

// ResolveTarget builds a Target from explicit flag values and the environment,
// applying flag > env > empty for each field independently.
//
// It does NOT default the workspace: an unset workspace stays "" and sdkclient
// maps "" -> "default" at construction (the single owner of that default). An
// empty Gateway is likewise left empty — a caller decision (e.g. doctor skips
// its online checks), never a silent fallback to the CLI's active gateway.
//
// Resolution is pure: getenv is injected (production passes os.Getenv, tests
// pass a map closure) so this package imports neither os nor any CLI framework.
func ResolveTarget(flagGateway, flagWorkspace string, getenv func(string) string) Target {
	return Target{
		Gateway:   resolveField(flagGateway, EnvGateway, getenv),
		Workspace: resolveField(flagWorkspace, EnvWorkspace, getenv),
	}
}

// resolveField applies flag > env > empty for one field. An empty flag value is
// treated as unset so the env var can take effect.
func resolveField(flagVal, envKey string, getenv func(string) string) string {
	if flagVal != "" {
		return flagVal
	}
	return getenv(envKey)
}
