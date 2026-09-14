package run

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPumpInteractiveSessionCopiesInputOutputAndResizes(t *testing.T) {
	session := newPumpSession("sandbox output")
	resize := make(chan os.Signal, 1)
	resize <- syscall.SIGWINCH
	var stdout bytes.Buffer

	if err := pumpInteractiveSession(context.Background(), session, bytes.NewBufferString("user input"), &stdout, resize, func() (uint32, uint32, error) {
		return 120, 40, nil
	}); err != nil {
		t.Fatalf("pumpInteractiveSession: %v", err)
	}
	input, sizes := session.snapshot()
	if stdout.String() != "sandbox output" || input != "user input" {
		t.Errorf("stdout=%q input=%q", stdout.String(), input)
	}
	if !reflect.DeepEqual(sizes, [][2]uint32{{120, 40}}) {
		t.Errorf("sizes = %+v", sizes)
	}
}

func TestCopyInteractiveFileStopsOnCancellation(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- copyInteractiveFile(ctx, io.Discard, input) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive input remained blocked after cancellation")
	}
}

type pumpSession struct {
	mu        sync.Mutex
	output    []byte
	input     bytes.Buffer
	sizes     [][2]uint32
	resized   chan struct{}
	inputDone chan struct{}
}

func newPumpSession(output string) *pumpSession {
	return &pumpSession{output: []byte(output), resized: make(chan struct{}), inputDone: make(chan struct{})}
}

func (s *pumpSession) Read(p []byte) (int, error) {
	<-s.resized
	<-s.inputDone
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.output) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.output)
	s.output = s.output[n:]
	return n, nil
}

func (s *pumpSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.input.Write(p)
	select {
	case <-s.inputDone:
	default:
		close(s.inputDone)
	}
	return n, err
}

func (s *pumpSession) Resize(cols, rows uint32) error {
	s.mu.Lock()
	s.sizes = append(s.sizes, [2]uint32{cols, rows})
	s.mu.Unlock()
	select {
	case <-s.resized:
	default:
		close(s.resized)
	}
	return nil
}

func (*pumpSession) ExitCode() (int, error) { return 0, nil }
func (*pumpSession) Close() error           { return nil }

func (s *pumpSession) snapshot() (string, [][2]uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.String(), append([][2]uint32(nil), s.sizes...)
}
