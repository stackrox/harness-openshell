package cmd

import "os"

var Version = "dev"

func resolveSandboxImage(agentImage string) string {
	if envImage := os.Getenv("HARNESS_OS_IMAGE"); envImage != "" {
		return envImage
	}
	if agentImage != "" {
		return agentImage
	}
	return versionedImage("sandbox")
}

func versionedImage(name string) string {
	base := "quay.io/rcochran/openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}
