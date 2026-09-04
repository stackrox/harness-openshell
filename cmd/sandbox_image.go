package cmd

import "os"

var Version = "dev"

// resolveSandboxImage determines which container image to use for sandbox execution.
//
// Image selection follows explicit precedence (highest to lowest):
//   1. HARNESS_OS_IMAGE environment variable - operator override for all contexts
//   2. agentImage parameter - workflow spec.sandbox.image explicit field
//   3. versionedImage("sandbox") - versioned default base image
//
// This precedence enables:
//   - Local development and debugging via environment variable
//   - Explicit per-workflow image overrides
//   - Consistent versioned defaults across contexts (local OpenShell, HyperShell)
//
// The resolved image must satisfy the Agent Runtime Contract (ARC):
// - Run as unprivileged 'sandbox' user
// - Provide writable Python virtualenv at /opt/agent/venv
// - Maintain standard PATH conventions for agent tools
// - Support multi-context execution (local, HyperShell personal, service-account)
func resolveSandboxImage(agentImage string) string {
	// 1. Environment override (highest priority)
	if envImage := os.Getenv("HARNESS_OS_IMAGE"); envImage != "" {
		return envImage
	}
	// 2. Explicit workflow image specification
	if agentImage != "" {
		return agentImage
	}
	// 3. Versioned default base image (fallback)
	return versionedImage("sandbox")
}

// versionedImage returns a fully qualified image reference for the named component.
// If Version is unset or "dev", returns the unversioned image (for local builds).
// Otherwise, appends the version tag to enable stable release references.
//
// Example outputs:
//   versionedImage("sandbox") with Version="dev" → quay.io/rcochran/openshell:sandbox
//   versionedImage("sandbox") with Version="0.1.0" → quay.io/rcochran/openshell:sandbox-0.1.0
func versionedImage(name string) string {
	base := "quay.io/rcochran/openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}
