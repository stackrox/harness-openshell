package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

// sandboxOpts holds the parameters that vary between callers of
// createSandbox (upLocal vs create). Everything else is derived
// from the agent config passed alongside.
type sandboxOpts struct {
	harnessDir string
	gw         gateway.Gateway
	name       string            // sandbox name
	image      string            // sandbox image ref or relative Dockerfile dir
	providers  []string          // registered providers to attach
	noTTY      bool              // true → TTY=false for the sandbox
	retrySleep time.Duration     // pause between retry attempts
	sandboxCmd []string          // command to run inside the sandbox
	payloadDir string            // pre-rendered payload dir to upload
	env        map[string]string // env vars injected via --env on sandbox create
	onSuccess  func(name string) // called after successful creation (optional)
}

// createSandbox resolves the image path, stages the payload directory,
// creates the sandbox with up to 5 retries, and cleans up on failure.
// Both upLocal and create delegate to this function after preparing
// their caller-specific sandboxOpts.
func createSandbox(opts sandboxOpts) error {
	// Relative Dockerfile dirs are resolved against harnessDir.
	image := opts.image
	if image != "" && !filepath.IsAbs(image) {
		candidate := filepath.Join(opts.harnessDir, image)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			image = candidate
		}
	}

	// Stage upload directory. openshell --upload copies the source directory
	// BY NAME into the destination, so we always create a subdirectory called
	// "openshell" and upload to /sandbox/.config → /sandbox/.config/openshell/*.
	tmpParent, err := os.MkdirTemp("", "harness-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(tmpParent)
	uploadDir := filepath.Join(tmpParent, "openshell")

	if err := os.Rename(opts.payloadDir, uploadDir); err != nil {
		return fmt.Errorf("staging payload: %w", err)
	}

	// Create sandbox with retry loop (up to 5 attempts).
	for attempt := 1; attempt <= 5; attempt++ {
		err := opts.gw.SandboxCreate(gateway.SandboxCreateOpts{
			Name:      opts.name,
			From:      image,
			Providers: opts.providers,
			TTY:       !opts.noTTY,
			Keep:      true,
			UploadSrc: uploadDir,
			UploadDst: "/sandbox/.config",
			Command:   opts.sandboxCmd,
			Env:       opts.env,
		})
		if err == nil {
			if opts.onSuccess != nil {
				opts.onSuccess(opts.name)
			}
			return nil
		}

		fmt.Printf("  Attempt %d failed: %v, retrying in 5s...\n", attempt, err)
		opts.gw.SandboxDelete(opts.name) // best-effort cleanup

		if attempt == 5 {
			return fmt.Errorf("sandbox create failed after 5 attempts: %w", err)
		}
		time.Sleep(opts.retrySleep)
	}
	return nil // unreachable but required by compiler
}
