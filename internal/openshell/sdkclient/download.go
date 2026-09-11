package sdkclient

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

const (
	maxDownloadBytes   = 256 << 20
	maxDownloadEntries = 10_000
)

// DownloadPath streams one file or directory from the sandbox to an absolute
// host path. It uses the SDK's authenticated SSH tunnel because the pinned Go
// SDK's higher-level file transport is not available yet.
func (c *client) DownloadPath(ctx context.Context, sandbox, sourcePath, destinationPath string) error {
	relative, err := sandboxRelativePath(sourcePath)
	if err != nil {
		return err
	}
	if destinationPath == "" || !filepath.IsAbs(destinationPath) {
		return errors.New("download destination must be an absolute host path")
	}
	if strings.IndexByte(destinationPath, 0) >= 0 {
		return errors.New("download destination must not contain a null byte")
	}
	if _, err := os.Lstat(destinationPath); err == nil {
		return fmt.Errorf("download destination already exists: %s", destinationPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect download destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o750); err != nil {
		return fmt.Errorf("create download destination parent: %w", err)
	}

	staging, err := os.MkdirTemp(filepath.Dir(destinationPath), ".harness-download-")
	if err != nil {
		return fmt.Errorf("create download staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	connection, err := c.openSSHSession(ctx, sandbox)
	if err != nil {
		return err
	}
	defer connection.Close()

	pipeReader, pipeWriter := io.Pipe()
	extractErr := make(chan error, 1)
	go func() {
		err := extractDownloadTar(pipeReader, staging, relative)
		if err != nil {
			_ = pipeReader.CloseWithError(err)
		}
		extractErr <- err
	}()

	var remoteError strings.Builder
	connection.session.Stdout = pipeWriter
	connection.session.Stderr = &remoteError
	command := "tar -cf - -C '/sandbox' -- " + shellQuote(relative)
	if err := connection.session.Start(command); err != nil {
		_ = pipeWriter.Close()
		<-extractErr
		return fmt.Errorf("start remote tar: %w", err)
	}
	waitErr := connection.session.Wait()
	_ = pipeWriter.Close()
	archiveErr := <-extractErr
	if archiveErr != nil {
		return fmt.Errorf("extracting sandbox output: %w", archiveErr)
	}
	if waitErr != nil {
		detail := strings.TrimSpace(remoteError.String())
		if strings.Contains(strings.ToLower(detail), "no such file") {
			return fmt.Errorf("%w: %s: %s", openshell.ErrNotFound, sourcePath, detail)
		}
		if detail != "" {
			return fmt.Errorf("remote tar: %w: %s", waitErr, detail)
		}
		return fmt.Errorf("remote tar: %w", waitErr)
	}

	stagedPath := filepath.Join(staging, filepath.FromSlash(relative))
	if !downloadPathWithin(staging, stagedPath) {
		return errors.New("download archive escaped staging directory")
	}
	if _, err := os.Lstat(stagedPath); err != nil {
		return fmt.Errorf("download archive did not contain %q: %w", sourcePath, err)
	}
	if err := os.Rename(stagedPath, destinationPath); err != nil {
		return fmt.Errorf("install downloaded output: %w", err)
	}
	return nil
}

func sandboxRelativePath(sourcePath string) (string, error) {
	if sourcePath == "" || !strings.HasPrefix(sourcePath, "/") {
		return "", errors.New("download source must be an absolute sandbox path")
	}
	if strings.IndexByte(sourcePath, 0) >= 0 {
		return "", errors.New("download source must not contain a null byte")
	}
	if strings.Contains(strings.ReplaceAll(sourcePath, "\\", "/"), "../") || strings.Contains(sourcePath, "/..") {
		return "", errors.New("download source must not contain '..' path segments")
	}
	clean := path.Clean(sourcePath)
	if clean == "/sandbox" || !strings.HasPrefix(clean, "/sandbox/") {
		return "", errors.New("download source must be below /sandbox")
	}
	return strings.TrimPrefix(clean, "/sandbox/"), nil
}

func extractDownloadTar(reader io.Reader, staging, expectedRoot string) error {
	tarReader := tar.NewReader(reader)
	entries := 0
	var totalBytes int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxDownloadEntries {
			return fmt.Errorf("download contains more than %d entries", maxDownloadEntries)
		}
		name := path.Clean(header.Name)
		if name == "." || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if name != expectedRoot && !strings.HasPrefix(name, expectedRoot+"/") {
			return fmt.Errorf("archive path %q is outside requested output", header.Name)
		}
		destination := filepath.Join(staging, filepath.FromSlash(name))
		if !downloadPathWithin(staging, destination) {
			return fmt.Errorf("archive path %q escapes staging directory", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0o750); err != nil {
				return fmt.Errorf("create output directory %q: %w", name, err)
			}
		case tar.TypeReg:
			if header.Size < 0 || totalBytes > maxDownloadBytes-header.Size {
				return fmt.Errorf("download exceeds %d-byte limit", maxDownloadBytes)
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
				return fmt.Errorf("create output parent %q: %w", name, err)
			}
			file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return fmt.Errorf("create output file %q: %w", name, err)
			}
			written, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("read output file %q: %w", name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close output file %q: %w", name, closeErr)
			}
			if written != header.Size {
				return fmt.Errorf("output file %q was truncated", name)
			}
			totalBytes += written
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("symbolic and hard links are not allowed in outputs: %q", name)
		default:
			return fmt.Errorf("unsupported output entry %q", name)
		}
	}
}

func downloadPathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
