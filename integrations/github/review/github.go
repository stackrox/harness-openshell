package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// GitHubGET lets gh resolve host authentication. No token is read into a task,
// argument, artifact, or workflow variable. The host only performs GET requests.
func GitHubGET(ctx context.Context, endpoint, accept string, limit int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var body bytes.Buffer
	out := &boundedWriter{writer: &body, remaining: limit, cancel: cancel}
	command := exec.CommandContext(ctx, "gh", "api", "--method", "GET", endpoint, "-H", "Accept: "+accept)
	command.Stdout, command.Stderr = out, io.Discard
	command.WaitDelay = time.Second
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("GitHub GET failed: %w", errors.Join(err, ctx.Err(), out.err))
	}
	if out.err != nil {
		return nil, out.err
	}
	return body.Bytes(), nil
}

type boundedWriter struct {
	mu        sync.Mutex
	writer    io.Writer
	remaining int
	cancel    context.CancelFunc
	err       error
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	allowed := len(data)
	if allowed > w.remaining {
		allowed = w.remaining
	}
	n, err := w.writer.Write(data[:allowed])
	w.remaining -= n
	if err == nil && n != len(data) {
		err = errors.New("review output exceeds size limit")
	}
	if err != nil {
		w.err = err
		w.cancel()
	}
	return n, err
}
