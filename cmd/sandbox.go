package cmd

import (
	"fmt"
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

// stagePayloadUpload moves the rendered payload dir into a staging directory
// named "openshell" so that `openshell --upload` copies it BY NAME into
// /sandbox/.config → /sandbox/.config/openshell/*. It returns the staged upload
// source dir and a cleanup func that removes the staging parent.
func stagePayloadUpload(payloadDir string) (uploadDir string, cleanup func(), err error) {
	tmpParent, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return "", nil, fmt.Errorf("creating staging dir: %w", err)
	}
	uploadDir = filepath.Join(tmpParent, "openshell")
	if err := os.Rename(payloadDir, uploadDir); err != nil {
		os.RemoveAll(tmpParent)
		return "", nil, fmt.Errorf("staging payload: %w", err)
	}
	return uploadDir, func() { os.RemoveAll(tmpParent) }, nil
}
