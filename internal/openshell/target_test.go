package openshell

import "testing"

// mapEnv returns a getenv closure backed by m, so ResolveTarget can be tested
// without touching the process environment (the function is pure).
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name          string
		flagGateway   string
		flagWorkspace string
		env           map[string]string
		wantGateway   string
		wantWorkspace string
	}{
		{
			name:          "flag wins over env",
			flagGateway:   "flag-gw",
			flagWorkspace: "flag-ws",
			env:           map[string]string{EnvGateway: "env-gw", EnvWorkspace: "env-ws"},
			wantGateway:   "flag-gw",
			wantWorkspace: "flag-ws",
		},
		{
			name:          "env wins when flag empty",
			flagGateway:   "",
			flagWorkspace: "",
			env:           map[string]string{EnvGateway: "env-gw", EnvWorkspace: "env-ws"},
			wantGateway:   "env-gw",
			wantWorkspace: "env-ws",
		},
		{
			name:          "empty flag and env stays empty (no defaulting)",
			flagGateway:   "",
			flagWorkspace: "",
			env:           map[string]string{},
			wantGateway:   "",
			wantWorkspace: "",
		},
		{
			name:          "flag wins when env empty",
			flagGateway:   "flag-gw",
			flagWorkspace: "flag-ws",
			env:           map[string]string{},
			wantGateway:   "flag-gw",
			wantWorkspace: "flag-ws",
		},
		{
			name:          "fields resolve independently",
			flagGateway:   "flag-gw",
			flagWorkspace: "",
			env:           map[string]string{EnvWorkspace: "env-ws"},
			wantGateway:   "flag-gw",
			wantWorkspace: "env-ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTarget(tt.flagGateway, tt.flagWorkspace, mapEnv(tt.env))
			if got.Gateway != tt.wantGateway {
				t.Errorf("Gateway = %q, want %q", got.Gateway, tt.wantGateway)
			}
			if got.Workspace != tt.wantWorkspace {
				t.Errorf("Workspace = %q, want %q", got.Workspace, tt.wantWorkspace)
			}
		})
	}
}
