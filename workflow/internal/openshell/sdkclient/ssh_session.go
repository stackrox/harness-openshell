package sdkclient

import (
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// sshSession is the short-lived SDK-routed SSH connection used for file
// transfer. The gateway relay remains the authentication boundary; the
// sandbox host key is intentionally not exposed by OpenShell's tunnel.
type sshSession struct {
	tunnel  io.ReadWriteCloser
	client  *ssh.Client
	session *ssh.Session
}

func (s *sshSession) Close() error {
	return errors.Join(s.session.Close(), s.client.Close(), s.tunnel.Close())
}

func (c *client) openSSHSession(ctx context.Context, sandbox string) (*sshSession, error) {
	tunnel, err := c.raw.SSH().Tunnel(ctx, c.workspace, sandbox, 22)
	if err != nil {
		return nil, fmt.Errorf("open SSH tunnel: %w", err)
	}

	connection, channels, requests, err := ssh.NewClientConn(&tunnelConn{ReadWriteCloser: tunnel}, "sandbox:22", &ssh.ClientConfig{
		User: "sandbox",
		// The authenticated gateway relay is the trust boundary. Tunnel does not
		// expose the sandbox host fingerprint, and the upstream CLI also disables
		// host-key checking for this scoped connection.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	})
	if err != nil {
		_ = tunnel.Close()
		return nil, fmt.Errorf("SSH handshake: %w", err)
	}
	sshClient := ssh.NewClient(connection, channels, requests)
	session, err := sshClient.NewSession()
	if err != nil {
		_ = sshClient.Close()
		_ = tunnel.Close()
		return nil, fmt.Errorf("open SSH session: %w", err)
	}
	return &sshSession{tunnel: tunnel, client: sshClient, session: session}, nil
}
