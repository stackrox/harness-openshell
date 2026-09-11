package sdkclient

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"golang.org/x/crypto/ssh"
)

func TestDownloadPathRegularFile(t *testing.T) {
	archive := tarArchive(t, func(tw *tar.Writer) {
		tarHeader(t, tw, "artifacts/report.txt", tar.TypeReg, 0o600, "report\n")
	})
	endpoint := &downloadSSH{t: t, archive: archive}
	client := newClient(&clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}, "team")
	destination := filepath.Join(t.TempDir(), "artifacts")

	if err := client.DownloadPath(context.Background(), "review", "/sandbox/artifacts/report.txt", filepath.Join(destination, "report.txt")); err != nil {
		t.Fatalf("DownloadPath: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "report\n" {
		t.Errorf("downloaded content = %q", data)
	}
	sandbox, port, command := endpoint.snapshot()
	if sandbox != "review" || port != 22 {
		t.Errorf("tunnel target = %s:%d", sandbox, port)
	}
	if want := "tar -cf - -C '/sandbox' -- 'artifacts/report.txt'"; command != want {
		t.Errorf("command = %q, want %q", command, want)
	}
}

func TestDownloadPathDirectoryPreservesShape(t *testing.T) {
	archive := tarArchive(t, func(tw *tar.Writer) {
		tarHeader(t, tw, "artifacts/", tar.TypeDir, 0o750, "")
		tarHeader(t, tw, "artifacts/nested/result.txt", tar.TypeReg, 0o600, "ok")
	})
	endpoint := &downloadSSH{t: t, archive: archive}
	client := newClient(&clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}, "team")
	destination := filepath.Join(t.TempDir(), "artifacts")

	if err := client.DownloadPath(context.Background(), "review", "/sandbox/artifacts", destination); err != nil {
		t.Fatalf("DownloadPath: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "nested", "result.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Errorf("downloaded content = %q", data)
	}
}

func TestDownloadPathReportsMissingRemotePath(t *testing.T) {
	endpoint := &downloadSSH{t: t, remoteErr: errors.New("tar: /sandbox/missing: No such file or directory")}
	client := newClient(&clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}, "team")
	err := client.DownloadPath(context.Background(), "review", "/sandbox/missing", filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, openshell.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestSandboxRelativePathRejectsEscapes(t *testing.T) {
	for _, source := range []string{"relative", "/tmp/file", "/sandbox", "/sandbox/../etc/passwd", "/sandbox/a/../../etc"} {
		if _, err := sandboxRelativePath(source); err == nil {
			t.Errorf("sandboxRelativePath(%q) unexpectedly succeeded", source)
		}
	}
	if got, err := sandboxRelativePath("/sandbox/foo/..bar"); err != nil || got != "foo/..bar" {
		t.Fatalf("sandboxRelativePath legitimate filename = %q, %v", got, err)
	}
}

func TestExtractDownloadTarRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name string
		add  func(*tar.Writer)
	}{
		{name: "traversal", add: func(tw *tar.Writer) { tarHeader(t, tw, "artifacts/../secret", tar.TypeReg, 0o600, "x") }},
		{name: "outside root", add: func(tw *tar.Writer) { tarHeader(t, tw, "other/file", tar.TypeReg, 0o600, "x") }},
		{name: "symlink", add: func(tw *tar.Writer) {
			header := &tar.Header{Name: "artifacts/link", Typeflag: tar.TypeSymlink, Linkname: "target"}
			if err := tw.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			archive := tarArchive(t, tc.add)
			if err := extractDownloadTar(bytes.NewReader(archive), t.TempDir(), "artifacts"); err == nil {
				t.Fatal("extractDownloadTar unexpectedly succeeded")
			}
		})
	}
}

func TestExtractDownloadTarEnforcesSizeLimit(t *testing.T) {
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	header := &tar.Header{Name: "artifacts/large", Mode: 0o600, Size: maxDownloadBytes + 1}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if err := extractDownloadTar(bytes.NewReader(archive.Bytes()), t.TempDir(), "artifacts"); err == nil {
		t.Fatal("extractDownloadTar unexpectedly succeeded")
	}
}

func tarArchive(t *testing.T, add func(*tar.Writer)) []byte {
	t.Helper()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	add(tw)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func tarHeader(t *testing.T, tw *tar.Writer, name string, typeflag byte, mode int64, data string) {
	t.Helper()
	header := &tar.Header{Name: name, Typeflag: typeflag, Mode: mode, Size: int64(len(data))}
	if typeflag == tar.TypeDir {
		header.Size = 0
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if typeflag == tar.TypeReg {
		if _, err := io.WriteString(tw, data); err != nil {
			t.Fatal(err)
		}
	}
}

type downloadSSH struct {
	v1.SSHInterface
	t         *testing.T
	archive   []byte
	remoteErr error

	mu      sync.Mutex
	sandbox string
	port    uint32
	command string
}

func (s *downloadSSH) Tunnel(_ context.Context, _ string, sandbox string, port uint32, _ ...v1.TunnelOption) (io.ReadWriteCloser, error) {
	s.mu.Lock()
	s.sandbox, s.port = sandbox, port
	s.mu.Unlock()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		defer listener.Close()
		serverConn, err := listener.Accept()
		if err != nil {
			s.t.Error(err)
			return
		}
		s.serve(serverConn)
	}()
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	return clientConn, nil
}

func (s *downloadSSH) snapshot() (sandbox string, port uint32, command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sandbox, s.port, s.command
}

func (s *downloadSSH) serve(conn net.Conn) {
	defer conn.Close()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		s.t.Error(err)
		return
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		s.t.Error(err)
		return
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		s.t.Error(err)
		return
	}
	go ssh.DiscardRequests(requests)
	newChannel, ok := <-channels
	if !ok {
		s.t.Error("SSH client opened no session channel")
		return
	}
	channel, channelRequests, err := newChannel.Accept()
	if err != nil {
		s.t.Error(err)
		return
	}
	defer channel.Close()
	for request := range channelRequests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			s.t.Error(err)
			return
		}
		_ = request.Reply(true, nil)
		s.mu.Lock()
		s.command = payload.Command
		s.mu.Unlock()
		if s.remoteErr == nil {
			_, _ = channel.Write(s.archive)
		} else {
			_, _ = fmt.Fprint(channel.Stderr(), s.remoteErr)
		}
		status := uint32(0)
		if s.remoteErr != nil {
			status = 1
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}
