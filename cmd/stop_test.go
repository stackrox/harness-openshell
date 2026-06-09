package cmd

import (
	"fmt"
	"testing"
)

type stopStartMockGW struct {
	mockGW
	sandboxNames []string
	stoppedNames []string
	startedNames []string
	stopErr      error
	startErr     error
}

func (m *stopStartMockGW) SandboxList() ([]string, error) { return m.sandboxNames, nil }
func (m *stopStartMockGW) SandboxStop(name string) error {
	m.stoppedNames = append(m.stoppedNames, name)
	return m.stopErr
}
func (m *stopStartMockGW) SandboxStart(name string) error {
	m.startedNames = append(m.startedNames, name)
	return m.startErr
}

func TestResolveSandboxName_Explicit(t *testing.T) {
	gw := &stopStartMockGW{}
	name, err := resolveSandboxName(gw, []string{"my-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-agent" {
		t.Errorf("got %q, want my-agent", name)
	}
}

func TestResolveSandboxName_AutoSingle(t *testing.T) {
	gw := &stopStartMockGW{sandboxNames: []string{"agent"}}
	name, err := resolveSandboxName(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name != "agent" {
		t.Errorf("got %q, want agent", name)
	}
}

func TestResolveSandboxName_AmbiguousError(t *testing.T) {
	gw := &stopStartMockGW{sandboxNames: []string{"a", "b"}}
	_, err := resolveSandboxName(gw, nil)
	if err == nil {
		t.Fatal("expected error for multiple sandboxes")
	}
}

func TestResolveSandboxName_NoneError(t *testing.T) {
	gw := &stopStartMockGW{}
	_, err := resolveSandboxName(gw, nil)
	if err == nil {
		t.Fatal("expected error for no sandboxes")
	}
}

func TestStop_Success(t *testing.T) {
	gw := &stopStartMockGW{sandboxNames: []string{"agent"}}
	name, _ := resolveSandboxName(gw, nil)
	if err := gw.SandboxStop(name); err != nil {
		t.Fatal(err)
	}
	if len(gw.stoppedNames) != 1 || gw.stoppedNames[0] != "agent" {
		t.Errorf("stopped = %v", gw.stoppedNames)
	}
}

func TestStop_Error(t *testing.T) {
	gw := &stopStartMockGW{stopErr: fmt.Errorf("sandbox not found")}
	if err := gw.SandboxStop("missing"); err == nil {
		t.Fatal("expected error")
	}
}
