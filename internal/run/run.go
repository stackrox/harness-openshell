package run

// Upload maps a host path to its destination inside the sandbox.
type Upload struct {
	Src string
	Dst string
}

// SandboxRunRequest carries the neutral vocabulary needed for SDK-native
// sandbox execution.
type SandboxRunRequest struct {
	Name      string
	Image     string
	Providers []string
	Env       map[string]string
	Command   []string
	Uploads   []Upload
	TTY       bool
	Keep      bool
	Policy    []byte
	Labels    map[string]string
}
