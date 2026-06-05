package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeStub(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	os.WriteFile(path, []byte(script), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return dir
}

func TestRunKubectl_Success(t *testing.T) {
	writeStub(t, `#!/bin/bash
echo "namespace/openshell created"
`)
	c := New("", "")
	out, err := c.RunKubectl(context.Background(), "create", "ns", "openshell")
	if err != nil {
		t.Fatalf("RunKubectl: %v", err)
	}
	if out != "namespace/openshell created" {
		t.Errorf("output = %q", out)
	}
}

func TestRunKubectl_Failure(t *testing.T) {
	writeStub(t, `#!/bin/bash
echo "error: not found" >&2
exit 1
`)
	c := New("", "")
	_, err := c.RunKubectl(context.Background(), "get", "ns", "nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}

func TestRunKubectl_RetryOnTransient(t *testing.T) {
	dir := t.TempDir()
	counterFile := filepath.Join(dir, "count")
	os.WriteFile(counterFile, []byte("0"), 0o644)

	writeStub(t, `#!/bin/bash
COUNT=$(cat `+counterFile+`)
COUNT=$((COUNT + 1))
echo $COUNT > `+counterFile+`
if [ $COUNT -lt 2 ]; then
  echo "connection refused" >&2
  exit 1
fi
echo "ok"
`)
	c := New("", "")
	out, err := c.RunKubectl(context.Background(), "get", "pods")
	if err != nil {
		t.Fatalf("RunKubectl: %v (should have retried)", err)
	}
	if out != "ok" {
		t.Errorf("output = %q", out)
	}
	data, _ := os.ReadFile(counterFile)
	if string(data) != "2\n" {
		t.Errorf("expected 2 attempts, got %s", data)
	}
}

func TestRunKubectl_NoRetryOnNonTransient(t *testing.T) {
	dir := t.TempDir()
	counterFile := filepath.Join(dir, "count")
	os.WriteFile(counterFile, []byte("0"), 0o644)

	writeStub(t, `#!/bin/bash
COUNT=$(cat `+counterFile+`)
COUNT=$((COUNT + 1))
echo $COUNT > `+counterFile+`
echo "resource not found" >&2
exit 1
`)
	c := New("", "")
	_, err := c.RunKubectl(context.Background(), "get", "secret", "missing")
	if err == nil {
		t.Error("expected error")
	}
	data, _ := os.ReadFile(counterFile)
	if string(data) != "1\n" {
		t.Errorf("expected 1 attempt (no retry), got %s", data)
	}
}

func TestRunKubectl_NamespaceInjection(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	c := New("", "openshell")
	c.RunKubectl(context.Background(), "get", "pods")
	data, _ := os.ReadFile(argsFile)
	args := string(data)
	if args != "-n openshell get pods\n" {
		t.Errorf("args = %q, expected namespace injection", args)
	}
}

func TestRunKubectl_KubeconfigInjection(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	writeStub(t, `#!/bin/bash
printf '%s\n' "$*" > `+argsFile+`
`)
	c := New("/path/to/kubeconfig", "")
	c.RunKubectl(context.Background(), "get", "ns")
	data, _ := os.ReadFile(argsFile)
	args := string(data)
	if args != "--kubeconfig /path/to/kubeconfig get ns\n" {
		t.Errorf("args = %q, expected kubeconfig injection", args)
	}
}

func TestApplyYAML(t *testing.T) {
	dir := t.TempDir()
	stdinFile := filepath.Join(dir, "stdin")
	writeStub(t, `#!/bin/bash
cat > `+stdinFile+`
`)
	c := New("", "test-ns")
	err := c.ApplyYAML(context.Background(), map[string]any{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata":   map[string]any{"name": "test-sa"},
	})
	if err != nil {
		t.Fatalf("ApplyYAML: %v", err)
	}
	data, _ := os.ReadFile(stdinFile)
	content := string(data)
	if !contains(content, "kind: ServiceAccount") {
		t.Errorf("YAML missing ServiceAccount: %s", content)
	}
	if !contains(content, "name: test-sa") {
		t.Errorf("YAML missing name: %s", content)
	}
}

func TestSecretExists(t *testing.T) {
	writeStub(t, `#!/bin/bash
[[ "$*" == *"my-secret"* ]] && exit 0
exit 1
`)
	c := New("", "default")
	if !c.SecretExists(context.Background(), "my-secret") {
		t.Error("expected secret to exist")
	}
	if c.SecretExists(context.Background(), "missing") {
		t.Error("expected secret to not exist")
	}
}

func TestIsTransient(t *testing.T) {
	if !isTransient("dial tcp: connection refused") {
		t.Error("connection refused should be transient")
	}
	if !isTransient("etcd leader changed") {
		t.Error("etcd leader changed should be transient")
	}
	if isTransient("resource not found") {
		t.Error("not found should NOT be transient")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != substr && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
