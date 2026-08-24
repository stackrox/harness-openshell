package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMinOpenShellVersionMatchesPin enforces the single source of truth: the
// MinOpenShellVersion constant must equal the repo-root .openshell-version pin
// that CI and `make openshell` install from. If this fails, someone bumped one
// without the other — bump both together when re-baselining.
func TestMinOpenShellVersionMatchesPin(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".openshell-version"))
	if err != nil {
		t.Fatalf("reading .openshell-version: %v", err)
	}
	pin := strings.TrimPrefix(strings.TrimSpace(string(data)), "v")
	if pin != MinOpenShellVersion {
		t.Errorf(".openshell-version pin %q != MinOpenShellVersion %q; bump both together", pin, MinOpenShellVersion)
	}
}
