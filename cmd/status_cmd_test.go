package cmd

import (
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
)

type statusMockGW struct {
	mockGW
	statusResult []gateway.SandboxInfo
	statusErr    error
	activeGW     string
	cliVer       string
}

func (m *statusMockGW) SandboxStatus() ([]gateway.SandboxInfo, error) {
	return m.statusResult, m.statusErr
}
func (m *statusMockGW) ActiveGateway() string { return m.activeGW }
func (m *statusMockGW) CLIVersion() string    { return m.cliVer }

func TestRunStatus_DisplaysSandboxes(t *testing.T) {
	gw := &statusMockGW{
		activeGW: "local",
		cliVer:   "openshell v0.0.58",
		statusResult: []gateway.SandboxInfo{
			{Name: "agent", Phase: "Ready"},
			{Name: "test", Phase: "Stopped"},
		},
	}
	if err := runStatus(gw); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
}

func TestRunStatus_NoSandboxes(t *testing.T) {
	gw := &statusMockGW{
		activeGW: "local",
		cliVer:   "openshell v0.0.58",
	}
	if err := runStatus(gw); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
}

func TestRunStatus_NoGateway(t *testing.T) {
	gw := &statusMockGW{}
	if err := runStatus(gw); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
}
