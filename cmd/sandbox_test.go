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
		workflow := &resolvedWorkflow{
			Desired: &config.Harness{Spec: config.Spec{Sandbox: config.Sandbox{Image: image}}},
			BaseDir: filepath.Dir(dir),
		}
		_, cleanup, err := buildRunRequest(workflow, "")
		cleanup()
		if err == nil || !strings.Contains(err.Error(), "local sandbox images are unsupported; use a registry image reference") {
			t.Errorf("buildRunRequest(%q) error = %v", image, err)
		}
	}
}

func TestCanonicalRunRequestKeepsRegistryReference(t *testing.T) {
	const image = "quay.io/stackrox/reviewer:v1"
	req, cleanup, err := buildRunRequest(&resolvedWorkflow{
		Desired: &config.Harness{Spec: config.Spec{Sandbox: config.Sandbox{Image: image}}},
		BaseDir: t.TempDir(),
	}, "")
	defer cleanup()
	if err != nil || req.Image != image {
		t.Fatalf("buildRunRequest() image = %q, %v", req.Image, err)
	}
}

func TestResolveSandboxImagePathFindsRelativeLocalContext(t *testing.T) {
	dir := t.TempDir()
	got := resolveSandboxImagePath(filepath.Base(dir), filepath.Dir(dir))
	if got != dir {
		t.Fatalf("resolveSandboxImagePath() = %q, want %q", got, dir)
	}
}

func TestResolveSandboxImageDefaultsToCommunityBase(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "")
	if got := resolveSandboxImage(""); got != defaultSandboxImage {
		t.Fatalf("resolveSandboxImage() = %q, want %q", got, defaultSandboxImage)
	}
}

func TestResolveSandboxImageHonorsEnvironmentOverride(t *testing.T) {
	t.Setenv("HARNESS_OS_IMAGE", "quay.io/stackrox/agent:v1")
	if got := resolveSandboxImage(""); got != "quay.io/stackrox/agent:v1" {
		t.Fatalf("resolveSandboxImage() = %q, want environment override", got)
	}
}
