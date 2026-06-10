package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockProviderChecker implements profile.ProviderChecker for testing.
type mockProviderChecker struct {
	providers map[string]bool
}

func (m *mockProviderChecker) ProviderGet(name string) error {
	if m.providers[name] {
		return nil
	}
	return fmt.Errorf("not found")
}

func TestKeepSandbox_Default(t *testing.T) {
	cfg := &Config{}
	if !cfg.KeepSandbox() {
		t.Error("KeepSandbox() = false, want true (default)")
	}
}

func TestKeepSandbox_Explicit(t *testing.T) {
	keepFalse := false
	cfg := &Config{Keep: &keepFalse}
	if cfg.KeepSandbox() {
		t.Error("KeepSandbox() = true, want false")
	}
}

func TestStageHarnessDir(t *testing.T) {
	dir := t.TempDir()
	harnessDir := filepath.Join(dir, "harness")

	if err := StageHarnessDir(harnessDir); err != nil {
		t.Fatalf("StageHarnessDir: %v", err)
	}

	stat, err := os.Stat(harnessDir)
	if err != nil {
		t.Fatalf("stat harness dir: %v", err)
	}
	if !stat.IsDir() {
		t.Error("expected directory to be created")
	}
}

func TestValidateProviders_AllRegistered(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{"github": true, "vertex-local": true}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local"}, gw)
	if len(reg) != 2 {
		t.Errorf("registered = %v, want 2", reg)
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want empty", missing)
	}
}

func TestValidateProviders_SomeMissing(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{"github": true}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local", "atlassian"}, gw)
	if len(reg) != 1 || reg[0] != "github" {
		t.Errorf("registered = %v", reg)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want 2 items", missing)
	}
}

func TestValidateProviders_NoneRegistered(t *testing.T) {
	gw := &mockProviderChecker{providers: map[string]bool{}}
	reg, missing := ValidateProviders([]string{"github", "vertex-local"}, gw)
	if len(reg) != 0 {
		t.Errorf("registered = %v, want empty", reg)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want 2 items", missing)
	}
}
