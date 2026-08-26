package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stackrox/harness-openshell/internal/gateway"
)

// fakeRunner is a fake SandboxRunner that records calls and returns scripted
// SandboxCreate errors (one per call, in order; exhausted => nil).
type fakeRunner struct {
	calls   []runnerCall
	creates []error
	nextIdx int
}

type runnerCall struct {
	method string
	name   string
	opts   gateway.SandboxCreateOpts
}

func (f *fakeRunner) SandboxCreate(opts gateway.SandboxCreateOpts) error {
	f.calls = append(f.calls, runnerCall{method: "SandboxCreate", name: opts.Name, opts: opts})
	if f.nextIdx >= len(f.creates) {
		return nil
	}
	err := f.creates[f.nextIdx]
	f.nextIdx++
	return err
}

func (f *fakeRunner) SandboxDelete(name string) error {
	f.calls = append(f.calls, runnerCall{method: "SandboxDelete", name: name})
	return nil
}

func (f *fakeRunner) methods() []string {
	ms := make([]string, len(f.calls))
	for i, c := range f.calls {
		ms[i] = c.method
	}
	return ms
}

func TestRunSandboxSuccess(t *testing.T) {
	gw := &fakeRunner{}
	req := SandboxRunRequest{Name: "test-sandbox", Image: "ubuntu:20.04"}

	if err := RunSandbox(context.Background(), gw, req); err != nil {
		t.Fatalf("RunSandbox failed: %v", err)
	}

	// Exactly one create, no deletes.
	if got, want := gw.methods(), []string{"SandboxCreate"}; !equalStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestRunSandboxRetryThenSucceed(t *testing.T) {
	gw := &fakeRunner{creates: []error{errors.New("transient 1"), errors.New("transient 2")}}
	req := SandboxRunRequest{Name: "test-sandbox", Image: "ubuntu:20.04"}

	if err := RunSandbox(context.Background(), gw, req); err != nil {
		t.Fatalf("RunSandbox failed: %v", err)
	}

	// 3 creates, a best-effort delete between each failed attempt.
	want := []string{"SandboxCreate", "SandboxDelete", "SandboxCreate", "SandboxDelete", "SandboxCreate"}
	if got := gw.methods(); !equalStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestRunSandboxExhaustRetries(t *testing.T) {
	gw := &fakeRunner{creates: []error{
		errors.New("f1"), errors.New("f2"), errors.New("f3"), errors.New("f4"), errors.New("f5"),
	}}
	req := SandboxRunRequest{Name: "test-sandbox", Image: "ubuntu:20.04"}

	err := RunSandbox(context.Background(), gw, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox create failed after 5 attempts") {
		t.Fatalf("error = %q, want it to mention 5 attempts", err)
	}
	// Wrapped cause is preserved.
	if !strings.Contains(err.Error(), "f5") {
		t.Fatalf("error = %q, want wrapped last cause f5", err)
	}

	// 5 creates + a delete after each failed attempt.
	want := []string{
		"SandboxCreate", "SandboxDelete", "SandboxCreate", "SandboxDelete",
		"SandboxCreate", "SandboxDelete", "SandboxCreate", "SandboxDelete",
		"SandboxCreate", "SandboxDelete",
	}
	if got := gw.methods(); !equalStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestContextCancellation(t *testing.T) {
	gw := &fakeRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RunSandbox(ctx, gw, SandboxRunRequest{Name: "test-sandbox", Image: "ubuntu:20.04"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(gw.calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(gw.calls))
	}
}

func TestToCreateOpts(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		req := SandboxRunRequest{
			Name:            "sandbox",
			Gateway:         "local",
			Workspace:       "ws1",
			Image:           "ubuntu:20.04",
			Providers:       []string{"p1", "p2"},
			NoAutoProviders: true,
			TTY:             true,
			Keep:            true,
			Env:             map[string]string{"KEY": "val"},
			Command:         []string{"bash"},
			Labels:          map[string]string{"id": "123"},
			PolicyPath:      "/tmp/policy.yaml",
		}
		opts := toCreateOpts(req)
		if opts.Name != "sandbox" || opts.Gateway != "local" || opts.Workspace != "ws1" {
			t.Fatalf("name/gateway/workspace mismatch: %+v", opts)
		}
		if opts.From != "ubuntu:20.04" {
			t.Fatalf("Image should map to From, got %q", opts.From)
		}
		if len(opts.Providers) != 2 || opts.Providers[0] != "p1" || !opts.NoAutoProviders {
			t.Fatalf("providers mismatch: %+v", opts)
		}
		if !opts.TTY || !opts.Keep {
			t.Fatalf("tty/keep mismatch: %+v", opts)
		}
		if opts.Policy != "/tmp/policy.yaml" {
			t.Fatalf("PolicyPath should map to Policy, got %q", opts.Policy)
		}
	})

	t.Run("empty policy path omits Policy", func(t *testing.T) {
		opts := toCreateOpts(SandboxRunRequest{Name: "sandbox", Image: "ubuntu"})
		if opts.Policy != "" {
			t.Fatalf("Policy should be empty, got %q", opts.Policy)
		}
	})
}

func TestKeepMapsToCreateFlagNoPostSuccessDelete(t *testing.T) {
	for _, keep := range []bool{true, false} {
		gw := &fakeRunner{}
		req := SandboxRunRequest{Name: "test-sandbox", Image: "ubuntu:20.04", Keep: keep}

		if err := RunSandbox(context.Background(), gw, req); err != nil {
			t.Fatalf("RunSandbox failed: %v", err)
		}
		// Keep flows to the create flag; success never triggers a delete.
		if got := gw.methods(); !equalStrings(got, []string{"SandboxCreate"}) {
			t.Fatalf("keep=%v: calls = %v, want [SandboxCreate]", keep, got)
		}
		if gw.calls[0].opts.Keep != keep {
			t.Fatalf("keep=%v: opts.Keep = %v", keep, gw.calls[0].opts.Keep)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
