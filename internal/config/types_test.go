package config

import "testing"

func TestInferenceTimeoutSecs(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    uint64
		wantErr bool
	}{
		{name: "empty is zero", timeout: "", want: 0},
		{name: "seconds", timeout: "60s", want: 60},
		{name: "minutes", timeout: "2m", want: 120},
		{name: "mixed", timeout: "1m30s", want: 90},
		{name: "sub-second rounds", timeout: "1500ms", want: 2},
		{name: "bare integer is an error", timeout: "60", wantErr: true},
		{name: "garbage is an error", timeout: "soon", wantErr: true},
		{name: "negative is an error", timeout: "-5s", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Inference{Timeout: tt.timeout}.TimeoutSecs()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %d", tt.timeout, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.timeout, err)
			}
			if got != tt.want {
				t.Errorf("TimeoutSecs(%q) = %d, want %d", tt.timeout, got, tt.want)
			}
		})
	}
}
