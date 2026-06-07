package k8s

import (
	"context"
	"fmt"
	"strings"
)

// MockRunner records calls for testing. Returns preconfigured responses.
type MockRunner struct {
	Calls     []string
	Responses map[string]string // command prefix → stdout response
	Errors    map[string]error  // command prefix → error
}

func NewMockRunner() *MockRunner {
	return &MockRunner{
		Responses: make(map[string]string),
		Errors:    make(map[string]error),
	}
}

func (m *MockRunner) record(args ...string) string {
	call := strings.Join(args, " ")
	m.Calls = append(m.Calls, call)
	return call
}

func (m *MockRunner) respond(call string) (string, error) {
	for prefix, err := range m.Errors {
		if strings.HasPrefix(call, prefix) {
			return "", err
		}
	}
	for prefix, resp := range m.Responses {
		if strings.HasPrefix(call, prefix) {
			return resp, nil
		}
	}
	return "", nil
}

func (m *MockRunner) RunKubectl(_ context.Context, args ...string) (string, error) {
	return m.respond(m.record(args...))
}

func (m *MockRunner) RunKubectlOpts(_ context.Context, opts KubectlOpts) (string, error) {
	return m.respond(m.record(opts.Args...))
}

func (m *MockRunner) RunKubectlQuiet(_ context.Context, args ...string) error {
	_, err := m.respond(m.record(args...))
	return err
}

func (m *MockRunner) RunKubectlPassthrough(_ context.Context, args ...string) error {
	_, err := m.respond(m.record(args...))
	return err
}

func (m *MockRunner) RunHelm(_ context.Context, args ...string) error {
	_, err := m.respond(m.record(append([]string{"helm"}, args...)...))
	return err
}

func (m *MockRunner) RunOC(_ context.Context, args ...string) error {
	_, err := m.respond(m.record(append([]string{"oc"}, args...)...))
	return err
}

func (m *MockRunner) ApplyYAML(_ context.Context, resources ...map[string]any) error {
	for _, r := range resources {
		kind, _ := r["kind"].(string)
		m.record("apply-yaml", kind)
	}
	return nil
}

func (m *MockRunner) SecretExists(_ context.Context, name string) bool {
	call := m.record("secret-exists", name)
	_, err := m.respond(call)
	return err == nil
}

func (m *MockRunner) GetSecretField(_ context.Context, secretName, field string) ([]byte, error) {
	call := m.record("get-secret-field", secretName, field)
	resp, err := m.respond(call)
	if err != nil {
		return nil, err
	}
	return []byte(resp), nil
}

func (m *MockRunner) GetJSONPath(_ context.Context, resource, jsonpath string) (string, error) {
	return m.respond(m.record("get-jsonpath", resource, jsonpath))
}

func (m *MockRunner) NamespaceExists(_ context.Context, ns string) bool {
	call := m.record("namespace-exists", ns)
	_, err := m.respond(call)
	return err == nil
}

func (m *MockRunner) GetServiceNodePort(_ context.Context, svcName string, containerPort int) (int, error) {
	call := m.record(fmt.Sprintf("get-nodeport %s %d", svcName, containerPort))
	resp, err := m.respond(call)
	if err != nil {
		return 0, err
	}
	if resp == "" {
		return 30080, nil // default test NodePort
	}
	var port int
	fmt.Sscanf(resp, "%d", &port)
	return port, nil
}

func (m *MockRunner) GetNodeInternalIP(_ context.Context) (string, error) {
	call := m.record("get-node-ip")
	resp, err := m.respond(call)
	if err != nil {
		return "", err
	}
	if resp == "" {
		return "172.18.0.2", nil // default test node IP
	}
	return resp, nil
}

// HasCall checks if any recorded call starts with the given prefix.
func (m *MockRunner) HasCall(prefix string) bool {
	for _, c := range m.Calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// CallCount returns how many calls start with the given prefix.
func (m *MockRunner) CallCount(prefix string) int {
	n := 0
	for _, c := range m.Calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// String returns a readable dump of all calls.
func (m *MockRunner) String() string {
	return fmt.Sprintf("MockRunner{%d calls: %v}", len(m.Calls), m.Calls)
}
