package cmd

import (
	"fmt"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
)

func TestTeardownK8s_NamespaceNotFound(t *testing.T) {
	kc := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Errors["namespace-exists"] = fmt.Errorf("not found")

	gw := &mockGW{}
	teardownK8s(gw, nil, kc, clusterRunner)

	if clusterRunner.HasCall("delete") {
		t.Error("should not attempt deletes when namespace does not exist")
	}
}

func TestTeardownK8s_FullCleanup(t *testing.T) {
	kc := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()

	deletedGateways := []string{}
	gw := &mockGW{
		gatewayListResult: []gateway.GatewayInfo{
			{Name: "openshell-remote-ocp", Endpoint: "https://gw.example.com", Active: true},
			{Name: "openshell", Endpoint: "https://127.0.0.1:17670"},
		},
		onGatewayRemove: func(name string) { deletedGateways = append(deletedGateways, name) },
	}

	teardownK8s(gw, nil, kc, clusterRunner)

	// Should helm uninstall
	if !kc.HasCall("helm uninstall") {
		t.Errorf("expected helm uninstall, calls: %v", kc.Calls)
	}

	// Should delete agent-sandbox-system namespace
	if !clusterRunner.HasCall("delete ns agent-sandbox-system") {
		t.Errorf("expected delete ns agent-sandbox-system, calls: %v", clusterRunner.Calls)
	}

	// Should delete secrets
	if kc.CallCount("delete secret") != 1 {
		t.Errorf("expected 1 secret delete, got %d: %v", kc.CallCount("delete secret"), kc.Calls)
	}

	// Should delete openshell namespace
	if !clusterRunner.HasCall("delete ns openshell") {
		t.Errorf("expected delete ns openshell, calls: %v", clusterRunner.Calls)
	}

	// Should remove non-local gateway only
	if len(deletedGateways) != 1 || deletedGateways[0] != "openshell-remote-ocp" {
		t.Errorf("deleted gateways = %v, want [openshell-remote-ocp]", deletedGateways)
	}
}
