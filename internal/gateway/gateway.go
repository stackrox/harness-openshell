package gateway

// Gateway abstracts all OpenShell gateway operations. The CLI implementation
// shells out to the openshell binary; a future gRPC implementation will call
// the gateway API directly.
type Gateway interface {
	// InferenceGet checks if the gateway is active and reachable.
	InferenceGet() error

	// ProviderGet checks if a provider is registered. Returns nil if it exists.
	ProviderGet(name string) error

	// ProviderList returns the names of all registered providers.
	ProviderList() ([]string, error)

	// SandboxCreate creates a new sandbox. Stdin/stdout/stderr are connected
	// for TTY mode.
	SandboxCreate(opts SandboxCreateOpts) error

	// SandboxDelete deletes a sandbox by name. Ignores "not found" errors.
	SandboxDelete(name string) error

	// SandboxConnect opens an interactive session to a running sandbox.
	// In CLI mode this replaces the process (exec syscall). In gRPC mode
	// this would use bidirectional streaming.
	SandboxConnect(name string) error

	// SandboxUpload copies local files into a running sandbox.
	SandboxUpload(name, localDir, remotePath string) error

	// SandboxExec runs a command inside a sandbox and waits for it to finish.
	SandboxExec(name string, command ...string) error
}

type SandboxCreateOpts struct {
	Name      string
	Image     string
	Providers []string
	TTY       bool
	Keep      bool
	UploadSrc string // local path (empty = no upload)
	UploadDst string // remote path
	Command   []string
}
