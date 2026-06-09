package cmd

import (
	"fmt"
	"testing"
)

func TestStart_Success(t *testing.T) {
	gw := &stopStartMockGW{sandboxNames: []string{"agent"}}
	name, _ := resolveSandboxName(gw, nil)
	if err := gw.SandboxStart(name); err != nil {
		t.Fatal(err)
	}
	if len(gw.startedNames) != 1 || gw.startedNames[0] != "agent" {
		t.Errorf("started = %v", gw.startedNames)
	}
}

func TestStart_Error(t *testing.T) {
	gw := &stopStartMockGW{startErr: fmt.Errorf("sandbox not found")}
	if err := gw.SandboxStart("missing"); err == nil {
		t.Fatal("expected error")
	}
}
