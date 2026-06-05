package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProviders_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.toml")
	os.WriteFile(path, []byte(`
[[providers]]
name = "github"
type = "openshell"
description = "GitHub"
required = true
inputs = [
  { key = "GITHUB_TOKEN", kind = "env", secret = true },
]

[[providers]]
name = "gws"
type = "custom"
description = "Google Workspace"
upstream = "https://github.com/NVIDIA/OpenShell/issues/1268"
inputs = []
`), 0o644)

	providers, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("got %d providers, want 2", len(providers))
	}
	if providers[0].Name != "github" || providers[0].Type != "openshell" {
		t.Errorf("providers[0] = %+v", providers[0])
	}
	if !providers[0].Required {
		t.Error("github should be required")
	}
	if len(providers[0].Inputs) != 1 || providers[0].Inputs[0].Kind != "env" {
		t.Errorf("github inputs = %+v", providers[0].Inputs)
	}
	if providers[1].Upstream != "https://github.com/NVIDIA/OpenShell/issues/1268" {
		t.Errorf("gws upstream = %q", providers[1].Upstream)
	}
}

func TestLoadProviders_Missing(t *testing.T) {
	_, err := LoadProviders("/nonexistent.toml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadProviders_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	os.WriteFile(path, []byte(`not valid {{{{`), 0o644)

	_, err := LoadProviders(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openshell.toml")
	os.WriteFile(path, []byte(`
providers = ["github", "vertex-local"]
providers-custom = ["gws"]

[upstream]
chart-version = "0.0.55"
`), 0o644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if len(cfg.Providers) != 2 {
		t.Errorf("providers = %v", cfg.Providers)
	}
	if len(cfg.ProvidersCustom) != 1 || cfg.ProvidersCustom[0] != "gws" {
		t.Errorf("providers-custom = %v", cfg.ProvidersCustom)
	}
	if cfg.Upstream.ChartVersion != "0.0.55" {
		t.Errorf("Upstream.ChartVersion = %q", cfg.Upstream.ChartVersion)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil for missing config")
	}
}

func TestEnabledProviders_WithConfig(t *testing.T) {
	all := []Provider{
		{Name: "github", Type: "openshell"},
		{Name: "vertex", Type: "openshell"},
		{Name: "gws", Type: "custom"},
	}
	cfg := &ConfigFile{
		Providers:       []string{"github"},
		ProvidersCustom: []string{"gws"},
	}
	enabled := EnabledProviders(all, cfg)
	if len(enabled) != 2 {
		t.Fatalf("got %d, want 2", len(enabled))
	}
	if enabled[0].Name != "github" || enabled[1].Name != "gws" {
		t.Errorf("enabled = %v", enabled)
	}
}

func TestEnabledProviders_NilConfig(t *testing.T) {
	all := []Provider{{Name: "a"}, {Name: "b"}}
	enabled := EnabledProviders(all, nil)
	if len(enabled) != 2 {
		t.Errorf("nil config should return all, got %d", len(enabled))
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		val  string
		show int
		want string
	}{
		{"super-secret-token", 4, "supe***"},
		{"abc", 4, "***"},
		{"", 4, "***"},
		{"abcdef", 4, "abcd***"},
		{"ab", 4, "***"},
	}
	for _, tt := range tests {
		got := MaskValue(tt.val, tt.show)
		if got != tt.want {
			t.Errorf("MaskValue(%q, %d) = %q, want %q", tt.val, tt.show, got, tt.want)
		}
	}
}

func TestFileMetadata_ADC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adc.json")
	os.WriteFile(path, []byte(`{"quota_project_id": "my-project", "type": "authorized_user"}`), 0o644)

	meta := FileMetadata(path)
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta["project"] != "my-project" {
		t.Errorf("project = %q", meta["project"])
	}
	if meta["type"] != "authorized_user" {
		t.Errorf("type = %q", meta["type"])
	}
}

func TestFileMetadata_GWS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client_secret.json")
	os.WriteFile(path, []byte(`{"installed": {"client_id": "1715999888.apps.googleusercontent.com"}}`), 0o644)

	meta := FileMetadata(path)
	if meta == nil {
		t.Fatal("expected metadata")
	}
	if meta["client_id"] != "1715***" {
		t.Errorf("client_id = %q, want masked", meta["client_id"])
	}
}

func TestFileMetadata_NotJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.txt")
	os.WriteFile(path, []byte("not json"), 0o644)

	meta := FileMetadata(path)
	if meta != nil {
		t.Errorf("expected nil for non-JSON, got %v", meta)
	}
}

func TestFileMetadata_Missing(t *testing.T) {
	meta := FileMetadata("/nonexistent.json")
	if meta != nil {
		t.Errorf("expected nil for missing file, got %v", meta)
	}
}

func TestCheckInput_EnvSet(t *testing.T) {
	t.Setenv("TEST_VAR", "hello")
	ok, detail := CheckInput(Input{Key: "TEST_VAR", Kind: "env"})
	if !ok {
		t.Error("expected ok")
	}
	if detail != "✓ local env: TEST_VAR=hello" {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_EnvMissing(t *testing.T) {
	ok, detail := CheckInput(Input{Key: "NONEXISTENT_VAR_XYZ", Kind: "env"})
	if ok {
		t.Error("expected not ok")
	}
	if detail != "✗ local env: NONEXISTENT_VAR_XYZ not set  →  export NONEXISTENT_VAR_XYZ=..." {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_EnvSecret(t *testing.T) {
	t.Setenv("SECRET_VAR", "super-secret-value")
	ok, detail := CheckInput(Input{Key: "SECRET_VAR", Kind: "env", Secret: true})
	if !ok {
		t.Error("expected ok")
	}
	if detail != "✓ local env: SECRET_VAR=supe***" {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("data"), 0o644)

	ok, detail := CheckInput(Input{Key: path, Kind: "file"})
	if !ok {
		t.Error("expected ok")
	}
	if detail != "✓ local file: "+path {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_FileMissing(t *testing.T) {
	ok, detail := CheckInput(Input{Key: "/nonexistent/file.json", Kind: "file"})
	if ok {
		t.Error("expected not ok")
	}
	if detail != "✗ local file: /nonexistent/file.json not found" {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_CheckPass(t *testing.T) {
	ok, detail := CheckInput(Input{Key: "true", Kind: "check"})
	if !ok {
		t.Error("expected ok")
	}
	if detail != "✓ check: true" {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckInput_CheckFail(t *testing.T) {
	ok, detail := CheckInput(Input{Key: "false", Kind: "check"})
	if ok {
		t.Error("expected not ok")
	}
	if detail != "✗ check: false" {
		t.Errorf("detail = %q", detail)
	}
}

func TestCheckProvider_AllPass(t *testing.T) {
	t.Setenv("GOOD_VAR", "yes")
	p := Provider{
		Name:   "test",
		Inputs: []Input{{Key: "GOOD_VAR", Kind: "env"}},
	}
	ok, details := CheckProvider(p)
	if !ok {
		t.Error("expected ok")
	}
	if len(details) != 1 {
		t.Errorf("details = %v", details)
	}
}

func TestCheckProvider_SomeFail(t *testing.T) {
	t.Setenv("SET_VAR", "yes")
	p := Provider{
		Name: "test",
		Inputs: []Input{
			{Key: "SET_VAR", Kind: "env"},
			{Key: "MISSING_VAR_XYZ", Kind: "env"},
		},
	}
	ok, details := CheckProvider(p)
	if ok {
		t.Error("expected not ok")
	}
	if len(details) != 2 {
		t.Errorf("details = %v", details)
	}
}
