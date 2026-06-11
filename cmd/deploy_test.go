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
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)
	return dir
}

func setupOCPGatewayConfig(t *testing.T, dir string) string {
	t.Helper()
	gwDir := filepath.Join(dir, "gateways", "ocp")
	os.MkdirAll(filepath.Join(gwDir, "helm"), 0o755)
	os.MkdirAll(filepath.Join(gwDir, "addons"), 0o755)
	os.WriteFile(filepath.Join(gwDir, "gateway.yaml"), []byte(`
gateway:
  type: remote
  platform: ocp
  service: route
  name: test-ocp
chart:
  oci: oci://ghcr.io/nvidia/openshell/helm-chart
  version: "0.0.58"
  crd:
    url: https://example.com/crd.yaml
helm:
  values: values.yaml
addons:
  manifests: [addons/rbac.yaml, addons/route.yaml]
ocp:
  scc-privileged: [sa1, sa2]
  scc-anyuid: [sa1]
secrets:
  names: [secret-a]
  mtls: test-client-tls
`), 0o644)
	os.WriteFile(filepath.Join(gwDir, "helm", "values.yaml"), []byte("image:\n  pullPolicy: Always\n"), 0o644)
	os.WriteFile(filepath.Join(gwDir, "addons", "rbac.yaml"), []byte("# rbac\n"), 0o644)
	os.WriteFile(filepath.Join(gwDir, "addons", "route.yaml"), []byte("# route\n"), 0o644)
	return gwDir
}

func setupK8sGatewayConfig(t *testing.T, dir string) string {
	t.Helper()
	gwDir := filepath.Join(dir, "gateways", "kind")
	os.MkdirAll(gwDir, 0o755)
	os.WriteFile(filepath.Join(gwDir, "gateway.yaml"), []byte(`
gateway:
  type: remote
  platform: k8s
  service: nodeport
  name: test-kind
  mode: direct
chart:
  oci: oci://ghcr.io/nvidia/openshell/helm-chart
  version: "0.0.58"
  crd:
    url: https://example.com/crd.yaml
`), 0o644)
	return gwDir
}

func TestDeployFromConfig_OCP_Success(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	gwDir := setupOCPGatewayConfig(t, dir)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.58")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")
	t.Setenv("HOME", t.TempDir())

	gwCfg, err := gateway.LoadConfig(gwDir)
	if err != nil {
		t.Fatal(err)
	}

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Responses["get-jsonpath"] = "apps.example.com"
	nsRunner.Responses["get-secret-field"] = "dGVzdA==" // base64 "test"

	gw := &mockGW{}

	err = deployFromConfig(dir, gwCfg, gw, nsRunner, clusterRunner)
	if err != nil {
		t.Fatalf("deployFromConfig: %v", err)
	}

	// Verify namespace created
	if !clusterRunner.HasCall("create ns openshell") {
		t.Errorf("missing create ns, calls: %v", clusterRunner.Calls)
	}

	// Verify CRD installed from config URL
	if !clusterRunner.HasCall("apply -f https://example.com/crd.yaml") {
		t.Errorf("missing CRD apply with config URL, calls: %v", clusterRunner.Calls)
	}

	// Verify addon manifests applied (2: rbac + route)
	if nsRunner.CallCount("apply -f") < 2 {
		t.Errorf("expected >=2 apply -f calls for addon manifests, got %d: %v", nsRunner.CallCount("apply -f"), nsRunner.Calls)
	}

	// Verify Helm install uses config chart OCI
	if !nsRunner.HasCall("helm upgrade --install openshell oci://ghcr.io/nvidia/openshell/helm-chart") {
		t.Errorf("missing helm install with config chart, calls: %v", nsRunner.Calls)
	}

	// Verify rollout status
	if !nsRunner.HasCall("rollout status statefulset/openshell") {
		t.Errorf("missing rollout status, calls: %v", nsRunner.Calls)
	}
}

func TestDeployFromConfig_K8s_NoSCCs(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	gwDir := setupK8sGatewayConfig(t, dir)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.58")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")
	t.Setenv("HOME", t.TempDir())

	gwCfg, err := gateway.LoadConfig(gwDir)
	if err != nil {
		t.Fatal(err)
	}

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()

	gw := &mockGW{}

	// NodePort deploy should succeed — mock returns default node IP + port
	err = deployFromConfig(dir, gwCfg, gw, nsRunner, clusterRunner)
	if err != nil {
		t.Fatalf("deployFromConfig: %v", err)
	}

	// Verify NO OC/SCC calls were made (k8s, not OCP)
	if nsRunner.HasCall("oc adm") {
		t.Errorf("should not run oc commands on k8s platform, calls: %v", nsRunner.Calls)
	}

	// Verify NO mTLS cert extraction (direct mode, no launcher)
	if nsRunner.HasCall("get-secret-field") {
		t.Errorf("should not extract mTLS certs for direct-mode k8s, calls: %v", nsRunner.Calls)
	}
}

func TestDeployFromConfig_HelmFailure(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	gwDir := setupOCPGatewayConfig(t, dir)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.58")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")

	gwCfg, err := gateway.LoadConfig(gwDir)
	if err != nil {
		t.Fatal(err)
	}

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Responses["get-jsonpath"] = "apps.example.com"
	nsRunner.Errors["helm upgrade"] = fmt.Errorf("chart not found")

	gw := &mockGW{}

	err = deployFromConfig(dir, gwCfg, gw, nsRunner, clusterRunner)
	if err == nil {
		t.Fatal("expected error from helm failure")
	}
	if nsRunner.HasCall("rollout status") {
		t.Error("should not attempt rollout after helm failure")
	}
}

func TestDeployFromConfig_CRDFailure(t *testing.T) {
	dir := setupDeployHarnessDir(t)
	gwDir := setupOCPGatewayConfig(t, dir)
	t.Setenv("OPENSHELL_CHART_VERSION", "0.0.58")
	t.Setenv("OPENSHELL_NAMESPACE", "openshell")

	gwCfg, err := gateway.LoadConfig(gwDir)
	if err != nil {
		t.Fatal(err)
	}

	nsRunner := k8s.NewMockRunner()
	clusterRunner := k8s.NewMockRunner()
	clusterRunner.Errors["apply -f"] = fmt.Errorf("network error")

	gw := &mockGW{}

	err = deployFromConfig(dir, gwCfg, gw, nsRunner, clusterRunner)
	if err == nil {
		t.Fatal("expected error from CRD install failure")
	}
	if nsRunner.HasCall("helm") {
		t.Error("should not attempt helm after CRD failure")
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
