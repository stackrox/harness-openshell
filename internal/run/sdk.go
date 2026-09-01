package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// RunSandboxSDK executes the SDK-native sandbox lifecycle. It intentionally
// has no retry loop: the SDK owns transport retries, while invalid requests,
// authentication failures, and agent failures must not be repeated blindly.
func RunSandboxSDK(ctx context.Context, client openshell.SandboxExecutionClient, req SandboxRunRequest, stdin io.Reader, stdout, stderr io.Writer) (runErr error) {
	_, err := client.CreateSandbox(ctx, openshell.SandboxCreate{
		Name:      req.Name,
		Image:     req.Image,
		Providers: req.Providers,
		Env:       req.Env,
		Labels:    req.Labels,
		Policy:    req.Policy,
	})
	if err != nil {
		return fmt.Errorf("creating sandbox %q: %w", req.Name, err)
	}
	if !req.Keep {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := client.DeleteSandbox(cleanupCtx, req.Name); err != nil {
				cleanupErr := fmt.Errorf("deleting sandbox %q: %w", req.Name, err)
				if runErr == nil {
					runErr = cleanupErr
				} else {
					runErr = errors.Join(runErr, cleanupErr)
				}
			}
		}()
	}

	if _, err := client.WaitSandboxReady(ctx, req.Name); err != nil {
		return fmt.Errorf("waiting for sandbox %q: %w", req.Name, err)
	}
	for _, upload := range req.Uploads {
		if err := client.UploadPath(ctx, req.Name, upload.Src, upload.Dst); err != nil {
			return fmt.Errorf("uploading %q to %q: %w", upload.Src, upload.Dst, err)
		}
	}
	if len(req.Command) == 0 {
		return nil
	}

	var exitCode int
	if req.TTY {
		exitCode, err = runInteractive(ctx, client, req.Name, req.Command, stdin, stdout)
	} else {
		exitCode, err = client.ExecSandbox(ctx, req.Name, req.Command, stdout, stderr)
	}
	if err != nil {
		return fmt.Errorf("executing command in sandbox %q: %w", req.Name, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("sandbox command exited with status %d", exitCode)
	}
	return nil
}
