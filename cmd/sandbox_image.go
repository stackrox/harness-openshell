package cmd

import "os"

func resolveSandboxImage(agentImage string) string {
	if envImage := os.Getenv("HARNESS_OS_IMAGE"); envImage != "" {
		return envImage
	}
	if agentImage != "" {
		return agentImage
	}
	return versionedImage("sandbox")
}
