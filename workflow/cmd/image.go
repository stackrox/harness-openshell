package cmd

import (
	"os"
	"path/filepath"
)

// Version is the build version, set at link time.
var Version = "dev"

const defaultSandboxImage = "ghcr.io/nvidia/openshell-community/sandboxes/base@sha256:aeef1c63f00e2913ea002ccb3aaf925f338b5c5d70e63576f0d95c16a138044e"

// resolveSandboxImagePath resolves a relative Dockerfile directory against
// harnessDir. An image ref (or an already-absolute path) is returned unchanged.
func resolveSandboxImagePath(image, harnessDir string) string {
	if image == "" || filepath.IsAbs(image) {
		return image
	}
	candidate := filepath.Join(harnessDir, image)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return image
}

func resolveSandboxImage(agentImage string) string {
	if envImage := os.Getenv("HARNESS_OS_IMAGE"); envImage != "" {
		return envImage
	}
	if agentImage != "" {
		return agentImage
	}
	return defaultSandboxImage
}
