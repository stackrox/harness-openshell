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
	"reflect"
	"strings"
	"sync"
	"testing"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"golang.org/x/crypto/ssh"
)

func TestUploadPathRegularFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("proof\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	endpoint := &uploadSSH{t: t}
	raw := &clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}
	client := newClient(raw, "team")
	if err := client.UploadPath(context.Background(), "review", source, "/sandbox/a'b/proof.txt"); err != nil {
		t.Fatalf("UploadPath: %v", err)
	}

	workspace, sandbox, port, command, archive := endpoint.snapshot()
	if workspace != "team" || sandbox != "review" || port != 22 {
		t.Errorf("tunnel target = %s/%s:%d", workspace, sandbox, port)
	}
	wantCommand := "mkdir -p -- '/sandbox/a'\\''b' && cat | tar xf - -C '/sandbox/a'\\''b'"
	if command != wantCommand {
		t.Errorf("command = %q, want %q", command, wantCommand)
	}
	entries := readTar(t, archive)
	want := []tarEntry{{name: "proof.txt", mode: 0o600, data: "proof\n"}}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("archive = %#v, want %#v", entries, want)
	}
}

func TestUploadPathProtectsOptionLikeDestination(t *testing.T) {
	source := filepath.Join(t.TempDir(), "payload")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	endpoint := &uploadSSH{t: t}
	raw := &clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}
	if err := newClient(raw, "team").UploadPath(context.Background(), "review", source, "-output"); err != nil {
		t.Fatalf("UploadPath: %v", err)
	}
	_, _, _, command, _ := endpoint.snapshot()
	if want := "mkdir -p -- '-output' && cat | tar xf - -C '-output'"; command != want {
		t.Errorf("command = %q, want %q", command, want)
	}
}

func TestWriteTarPreservesDirectoryShape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	if err := writeTar(&archive, root, "repo"); err != nil {
		t.Fatalf("writeTar: %v", err)
	}
	entries := readTar(t, archive.Bytes())
	rootInfo, _ := os.Lstat(root)
	emptyInfo, _ := os.Lstat(filepath.Join(root, "empty"))
	linkInfo, _ := os.Lstat(filepath.Join(root, "link"))
	want := []tarEntry{
		{name: "repo/", mode: int64(rootInfo.Mode().Perm()), directory: true},
		{name: "repo/a.txt", mode: 0o600, data: "a"},
		{name: "repo/b.txt", mode: 0o640, data: "b"},
		{name: "repo/empty/", mode: int64(emptyInfo.Mode().Perm()), directory: true},
		{name: "repo/link", mode: int64(linkInfo.Mode().Perm()), link: "a.txt"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("archive = %#v, want %#v", entries, want)
	}
}

func TestWriteTarRejectsUnsupportedFileType(t *testing.T) {
	socketFile, err := os.CreateTemp("/tmp", "harness-s2-socket-")
	if err != nil {
		t.Fatal(err)
	}
	socketPath := socketFile.Name()
	if err := socketFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(socketPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := writeTar(io.Discard, socketPath, "socket"); err == nil || !strings.Contains(err.Error(), "unsupported file type") {
		t.Fatalf("error = %v", err)
	}
}

func TestUploadTargetMatchesCLIPathSemantics(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		destination string
		mode        os.FileMode
		directory   string
		archiveName string
	}{
		{name: "exact file", source: "/host/source.txt", destination: "/sandbox/proof.txt", mode: 0o600, directory: "/sandbox", archiveName: "proof.txt"},
		{name: "file into directory", source: "/host/source.txt", destination: "/sandbox/", mode: 0o600, directory: "/sandbox/", archiveName: "source.txt"},
		{name: "relative exact file", source: "/host/source.txt", destination: "proof.txt", mode: 0o600, directory: ".", archiveName: "proof.txt"},
		// OpenShell treats a root-level destination as a directory because the
		// sandbox user cannot write directly to the filesystem root.
		{name: "root is directory", source: "/host/source.txt", destination: "/proof.txt", mode: 0o600, directory: "/proof.txt", archiveName: "source.txt"},
		{name: "directory retains basename", source: "/host/repo", destination: "/sandbox", mode: os.ModeDir | 0o755, directory: "/sandbox", archiveName: "repo"},
		{name: "symlink is file-like", source: "/host/source", destination: "/sandbox/link", mode: os.ModeSymlink | 0o777, directory: "/sandbox", archiveName: "link"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directory, archiveName, err := uploadTarget(tc.source, tc.destination, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if directory != tc.directory || archiveName != tc.archiveName {
				t.Errorf("target = (%q, %q), want (%q, %q)", directory, archiveName, tc.directory, tc.archiveName)
			}
		})
	}
}

func TestUploadPathReportsTunnelAndRemoteErrors(t *testing.T) {
	t.Run("tunnel", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "source")
		if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		raw := &clientWithSSH{ClientInterface: fake.NewClient(), ssh: failingSSH{err: errors.New("relay unavailable")}}
		err := newClient(raw, "team").UploadPath(context.Background(), "review", source, "/sandbox/source")
		if err == nil || !strings.Contains(err.Error(), "open SSH tunnel: relay unavailable") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("remote tar", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "source")
		if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		endpoint := &uploadSSH{t: t, remoteErr: errors.New("disk full")}
		raw := &clientWithSSH{ClientInterface: fake.NewClient(), ssh: endpoint}
		err := newClient(raw, "team").UploadPath(context.Background(), "review", source, "/sandbox/source")
		if err == nil || !strings.Contains(err.Error(), "remote tar") || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("error = %v", err)
		}
	})
}

type tarEntry struct {
	name      string
	mode      int64
	data      string
	link      string
	directory bool
}

func readTar(t *testing.T, data []byte) []tarEntry {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	var entries []tarEntry
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, tarEntry{
			name: header.Name, mode: header.Mode, data: string(body), link: header.Linkname,
			directory: header.Typeflag == tar.TypeDir,
		})
	}
}

type clientWithSSH struct {
	v1.ClientInterface
	ssh v1.SSHInterface
}

func (c *clientWithSSH) SSH() v1.SSHInterface { return c.ssh }

type failingSSH struct {
	v1.SSHInterface
	err error
}

func (s failingSSH) Tunnel(context.Context, string, string, uint32, ...v1.TunnelOption) (io.ReadWriteCloser, error) {
	return nil, s.err
}

type uploadSSH struct {
	v1.SSHInterface
	t         *testing.T
	remoteErr error

	mu        sync.Mutex
	workspace string
	sandbox   string
	port      uint32
	command   string
	archive   []byte
}

func (s *uploadSSH) Tunnel(_ context.Context, workspace, sandbox string, port uint32, _ ...v1.TunnelOption) (io.ReadWriteCloser, error) {
	s.mu.Lock()
	s.workspace, s.sandbox, s.port = workspace, sandbox, port
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
		listener.Close()
		return nil, err
	}
	return clientConn, nil
}

func (s *uploadSSH) snapshot() (workspace, sandbox string, port uint32, command string, archive []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspace, s.sandbox, s.port, s.command, append([]byte(nil), s.archive...)
}

func (s *uploadSSH) serve(conn net.Conn) {
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
		archive, err := io.ReadAll(channel)
		if err != nil {
			s.t.Error(err)
			return
		}
		s.mu.Lock()
		s.command = payload.Command
		s.archive = archive
		s.mu.Unlock()
		status := uint32(0)
		if s.remoteErr != nil {
			status = 1
			_, _ = fmt.Fprint(channel.Stderr(), s.remoteErr)
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}
