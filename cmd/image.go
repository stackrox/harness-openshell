package cmd

import (
	"os"
	"path/filepath"
)

// Version is the build version, set at link time and used to tag versioned
// sandbox images.
var Version = "dev"

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
	return versionedImage("sandbox")
}

func versionedImage(name string) string {
	base := "quay.io/rcochran/openshell"
	if Version == "" || Version == "dev" {
		return base + ":" + name
	}
	return base + ":" + name + "-" + Version
}
