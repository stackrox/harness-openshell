package sdkclient

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func (c *client) UploadPath(ctx context.Context, sandbox, sourcePath, destinationPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect source: %w", err)
	}
	destinationDir, archiveName, err := uploadTarget(sourcePath, destinationPath, info.Mode())
	if err != nil {
		return err
	}

	tunnel, err := c.raw.SSH().Tunnel(ctx, c.workspace, sandbox, 22)
	if err != nil {
		return fmt.Errorf("open SSH tunnel: %w", err)
	}
	defer tunnel.Close()

	connection, channels, requests, err := ssh.NewClientConn(&tunnelConn{ReadWriteCloser: tunnel}, "sandbox:22", &ssh.ClientConfig{
		User: "sandbox",
		// The authenticated gateway relay is the trust boundary. Tunnel does not
		// expose the sandbox host fingerprint, and the upstream CLI also disables
		// host-key checking for this scoped connection.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	})
	if err != nil {
		return fmt.Errorf("SSH handshake: %w", err)
	}
	sshClient := ssh.NewClient(connection, channels, requests)
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("open SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("open SSH stdin: %w", err)
	}
	var remoteError bytes.Buffer
	session.Stderr = &remoteError
	quotedDir := shellQuote(destinationDir)
	if err := session.Start("mkdir -p -- " + quotedDir + " && cat | tar xf - -C " + quotedDir); err != nil {
		return fmt.Errorf("start remote tar: %w", err)
	}
	if err := writeTar(stdin, sourcePath, archiveName); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("archive source: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close archive stream: %w", err)
	}
	if err := session.Wait(); err != nil {
		if detail := strings.TrimSpace(remoteError.String()); detail != "" {
			return fmt.Errorf("remote tar: %w: %s", err, detail)
		}
		return fmt.Errorf("remote tar: %w", err)
	}
	return nil
}

func uploadTarget(sourcePath, destinationPath string, mode os.FileMode) (directory, archiveName string, err error) {
	if destinationPath == "" {
		return "", "", errors.New("destination path is required")
	}
	if strings.IndexByte(destinationPath, 0) >= 0 {
		return "", "", errors.New("destination path contains a null byte")
	}
	fileLike := mode.IsRegular() || mode&os.ModeSymlink != 0
	if fileLike && !strings.HasSuffix(destinationPath, "/") {
		if slash := strings.LastIndexByte(destinationPath, '/'); slash >= 0 {
			parent := destinationPath[:slash]
			if parent == "" {
				parent = "/"
			}
			// OpenShell treats root-level file targets as directories rather than
			// attempting to create a file directly under the filesystem root.
			if parent != "/" {
				return parent, destinationPath[slash+1:], nil
			}
		} else {
			return ".", destinationPath, nil
		}
	}

	base := filepath.Base(filepath.Clean(sourcePath))
	if base == "." || base == string(filepath.Separator) {
		base = "."
	}
	return destinationPath, base, nil
}

func writeTar(w io.Writer, sourcePath, archiveName string) error {
	tw := tar.NewWriter(w)
	if err := appendTarPath(tw, sourcePath, filepath.ToSlash(archiveName)); err != nil {
		_ = tw.Close()
		return err
	}
	return tw.Close()
}

func appendTarPath(tw *tar.Writer, localPath, archivePath string) error {
	info, err := os.Lstat(localPath)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", localPath, err)
	}
	if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("unsupported file type %q", localPath)
	}

	var link string
	if info.Mode()&os.ModeSymlink != 0 {
		link, err = os.Readlink(localPath)
		if err != nil {
			return fmt.Errorf("read symlink %q: %w", localPath, err)
		}
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return fmt.Errorf("archive %q: %w", localPath, err)
	}
	header.Name = archivePath
	if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
		header.Name += "/"
	}
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("archive %q: %w", localPath, err)
	}

	switch {
	case info.Mode().IsRegular():
		file, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("open %q: %w", localPath, err)
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("archive %q: %w", localPath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %q: %w", localPath, closeErr)
		}
	case info.IsDir():
		entries, err := os.ReadDir(localPath)
		if err != nil {
			return fmt.Errorf("read directory %q: %w", localPath, err)
		}
		for _, entry := range entries {
			childArchivePath := filepath.ToSlash(filepath.Join(archivePath, entry.Name()))
			if err := appendTarPath(tw, filepath.Join(localPath, entry.Name()), childArchivePath); err != nil {
				return err
			}
		}
	case info.Mode()&os.ModeSymlink != 0:
		return nil
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

type tunnelConn struct {
	io.ReadWriteCloser
}

func (*tunnelConn) LocalAddr() net.Addr              { return tunnelAddr("local") }
func (*tunnelConn) RemoteAddr() net.Addr             { return tunnelAddr("sandbox:22") }
func (*tunnelConn) SetDeadline(time.Time) error      { return errors.ErrUnsupported }
func (*tunnelConn) SetReadDeadline(time.Time) error  { return errors.ErrUnsupported }
func (*tunnelConn) SetWriteDeadline(time.Time) error { return errors.ErrUnsupported }

type tunnelAddr string

func (tunnelAddr) Network() string  { return "openshell-tunnel" }
func (a tunnelAddr) String() string { return string(a) }
