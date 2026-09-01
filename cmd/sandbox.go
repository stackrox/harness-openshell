package cmd

import (
	"os"
	"path/filepath"
)

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
