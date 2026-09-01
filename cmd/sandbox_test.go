package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
)

func TestCanonicalRunRequestRejectsLocalImages(t *testing.T) {
	dir := t.TempDir()
	for _, image := range []string{dir, filepath.Base(dir)} {
		workflow := &canonicalWorkflow{
			Desired: &config.Harness{Spec: config.Spec{Sandbox: config.Sandbox{Image: image}}},
			BaseDir: filepath.Dir(dir),
		}
		_, cleanup, err := canonicalRunRequest(workflow)
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "local sandbox images are unsupported; use a registry image reference") {
			t.Errorf("canonicalRunRequest(%q) error = %v", image, err)
		}
	}
}

func TestCanonicalRunRequestKeepsRegistryReference(t *testing.T) {
	const image = "quay.io/stackrox/reviewer:v1"
	req, cleanup, err := canonicalRunRequest(&canonicalWorkflow{
		Desired: &config.Harness{Spec: config.Spec{Sandbox: config.Sandbox{Image: image}}},
		BaseDir: t.TempDir(),
	})
	defer cleanup()
	if err != nil || req.Image != image {
		t.Fatalf("canonicalRunRequest() image = %q, %v", req.Image, err)
	}
}

func TestResolveSandboxImagePathPreservesLegacyLocalContext(t *testing.T) {
	dir := t.TempDir()
	got := resolveSandboxImagePath(filepath.Base(dir), filepath.Dir(dir))
	if got != dir {
		t.Fatalf("resolveSandboxImagePath() = %q, want %q", got, dir)
	}
}
