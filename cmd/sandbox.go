package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/profile"
)

// sandboxOpts holds the parameters that vary between callers of
// createSandbox (upLocal vs createDirect). Everything else is
// derived from the profile.Config passed alongside.
type sandboxOpts struct {
	harnessDir string
	gw         gateway.Gateway
	cfg        *profile.Config
	providers  []string          // registered providers to attach
	noTTY      bool              // true → TTY=false for the sandbox
	retrySleep time.Duration     // pause between retry attempts
	sandboxCmd []string          // command to run inside the sandbox
	payloadDir string            // pre-rendered payload dir; skips StageHarnessDir when set
	onSuccess  func(name string) // called after successful creation (optional)
}

// createSandbox resolves the image path, stages the harness directory,
// creates the sandbox with up to 5 retries, and cleans up on failure.
// Both upLocal and createDirect delegate to this function after
// preparing their caller-specific sandboxOpts.
func createSandbox(opts sandboxOpts) error {
	cfg := opts.cfg

	// Resolve image path: SANDBOX_IMAGE env overrides profile; relative
	// Dockerfile dirs are resolved against harnessDir.
	if envImage := os.Getenv("SANDBOX_IMAGE"); envImage != "" {
		cfg.From = envImage
	} else if cfg.From != "" && !filepath.IsAbs(cfg.From) {
		candidate := filepath.Join(opts.harnessDir, cfg.From)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			cfg.From = candidate
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

	if opts.payloadDir != "" {
		if err := os.Rename(opts.payloadDir, uploadDir); err != nil {
			return fmt.Errorf("staging payload: %w", err)
		}
	} else {
		if err := profile.StageHarnessDir(cfg, uploadDir); err != nil {
			return fmt.Errorf("staging files: %w", err)
		}
	}

	// Create sandbox with retry loop (up to 5 attempts).
	for attempt := 1; attempt <= 5; attempt++ {
		err := opts.gw.SandboxCreate(gateway.SandboxCreateOpts{
			Name:      cfg.Name,
			From:      cfg.From,
			Providers: opts.providers,
			TTY:       !opts.noTTY,
			Keep:      cfg.KeepSandbox(),
			UploadSrc: uploadDir,
			UploadDst: "/sandbox/.config",
			Command:   opts.sandboxCmd,
		})
		if err == nil {
			if opts.onSuccess != nil {
				opts.onSuccess(cfg.Name)
			}
			return nil
		}

		fmt.Printf("  Attempt %d failed: %v, retrying in 5s...\n", attempt, err)
		opts.gw.SandboxDelete(cfg.Name) // best-effort cleanup

		if attempt == 5 {
			return fmt.Errorf("sandbox create failed after 5 attempts: %w", err)
		}
		time.Sleep(opts.retrySleep)
	}
	return nil // unreachable but required by compiler
}
