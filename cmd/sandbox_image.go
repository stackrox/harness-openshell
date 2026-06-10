package cmd

import "os"

// resolveSandboxImage returns the final sandbox image path following
// the precedence: SANDBOX_IMAGE env var > agentImage > defaultSandboxImage().
func resolveSandboxImage(agentImage string) string {
	if envImage := os.Getenv("SANDBOX_IMAGE"); envImage != "" {
		return envImage
	}
	if agentImage != "" {
		return agentImage
	}
	return defaultSandboxImage()
}
