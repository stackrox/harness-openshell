package run

// Upload maps a host path to its destination inside the sandbox.
type Upload struct {
	Src string
	Dst string
}

// Download maps a sandbox path to a path on the host output directory.
type Download struct {
	Src      string
	Dst      string
	Required bool
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
	Downloads []Download
	TTY       bool
	Keep      bool
	Policy    []byte
	Labels    map[string]string
}
