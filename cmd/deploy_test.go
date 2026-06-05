package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/k8s"
)

func setupDeployHarnessDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "values-ocp.yaml"), []byte("image:\n  pullPolicy: Always\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "profiles"), 0o755)
	return dir
}

func TestDeployRemote_Success(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.55")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")
	t.Setenv("HOME", t.TempDir())

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Responses["get-jsonpath"] = "apps.example.com"
	nsRunner.Responses["get-secret-field"] = "dGVzdA==" // base64 "test"
	nsRunner.Errors["get route"] = fmt.Errorf("not found") // trigger Route creation

	gw := &mockGW{}

	err := deployRemote(dir, gw, nsRunner, clusterRunner)
	if err != nil {
		t.Fatalf("deployRemote: %v", err)
	}

	// Verify namespace created
	if !clusterRunner.HasCall("create ns openshell") {
		t.Errorf("missing create ns, calls: %v", clusterRunner.Calls)
	}

	// Verify namespace labeled
	if !clusterRunner.HasCall("label ns openshell") {
		t.Errorf("missing label ns, calls: %v", clusterRunner.Calls)
	}

	// Verify CRD installed
	if !clusterRunner.HasCall("apply -f") {
		t.Errorf("missing CRD apply, calls: %v", clusterRunner.Calls)
	}

	// Verify RBAC applied (via kubectl apply -f)
	if !nsRunner.HasCall("apply -f") {
		t.Errorf("missing RBAC apply, calls: %v", nsRunner.Calls)
	}

	// Verify Helm install
	if !nsRunner.HasCall("helm upgrade --install") {
		t.Errorf("missing helm install, calls: %v", nsRunner.Calls)
	}

	// Verify rollout status
	if !nsRunner.HasCall("rollout status statefulset/openshell") {
		t.Errorf("missing rollout status, calls: %v", nsRunner.Calls)
	}

	// Verify Route applied (via kubectl apply -f)
	if nsRunner.CallCount("apply -f") < 2 {
		t.Errorf("expected 2 apply -f calls (RBAC + Route), got %d: %v", nsRunner.CallCount("apply -f"), nsRunner.Calls)
	}
}

func TestDeployRemote_HelmFailure(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.55")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Responses["get-jsonpath"] = "apps.example.com"
	nsRunner.Errors["helm upgrade"] = fmt.Errorf("chart not found")

	gw := &mockGW{}

	err := deployRemote(dir, gw, nsRunner, clusterRunner)
	if err == nil {
		t.Fatal("expected error from helm failure")
	}
	if !nsRunner.HasCall("helm upgrade") {
		t.Errorf("helm should have been called, calls: %v", nsRunner.Calls)
	}
	// Should NOT have attempted rollout after helm failure
	if nsRunner.HasCall("rollout status") {
		t.Error("should not attempt rollout after helm failure")
	}
}

func TestDeployRemote_CRDFailure(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.55")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Errors["apply -f"] = fmt.Errorf("network error")

	gw := &mockGW{}

	err := deployRemote(dir, gw, nsRunner, clusterRunner)
	if err == nil {
		t.Fatal("expected error from CRD install failure")
	}
	// Should NOT have attempted helm after CRD failure
	if nsRunner.HasCall("helm") {
		t.Error("should not attempt helm after CRD failure")
	}
}

func TestDeployRemote_NoAppsDomain(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.55")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Responses["get-jsonpath"] = "" // empty domain

	gw := &mockGW{}

	err := deployRemote(dir, gw, nsRunner, clusterRunner)
	if err == nil {
		t.Fatal("expected error for missing apps domain")
	}
}

func TestDeployLocal_NoGateway(t *testing.T) {
	gw := &mockGW{inferenceErr: fmt.Errorf("not reachable")}
	gw.gatewayListResult = []gateway.GatewayInfo{
		{Name: "local", Endpoint: "https://127.0.0.1:17670"},
	}

	err := deployLocal(gw)
	if err == nil {
		t.Fatal("expected error for unreachable gateway")
	}
}
