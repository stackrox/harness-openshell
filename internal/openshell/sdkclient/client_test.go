package sdkclient

import (
	"context"
	"errors"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name          string
		healthy       bool
		version       string
		expectHealthy bool
		expectVersion string
	}{
		{
			name:          "healthy gateway",
			healthy:       true,
			version:       "9.9.9",
			expectHealthy: true,
			expectVersion: "9.9.9",
		},
		{
			name:          "unhealthy gateway",
			healthy:       false,
			version:       "8.8.8",
			expectHealthy: false,
			expectVersion: "8.8.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fc := fake.NewClient(fake.WithHealthResult(&types.HealthResult{
				Healthy: tt.healthy,
				Version: tt.version,
			}))
			c := NewFromClient(fc, "default")

			h, err := c.Health(ctx)
			if err != nil {
				t.Fatalf("Health() returned unexpected error: %v", err)
			}
			if h.Healthy != tt.expectHealthy {
				t.Errorf("expected Healthy=%v, got %v", tt.expectHealthy, h.Healthy)
			}
			if h.Version != tt.expectVersion {
				t.Errorf("expected Version=%q, got %q", tt.expectVersion, h.Version)
			}
		})
	}
}

func TestHealthErrorTranslated(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.Close()

	c := NewFromClient(fc, "default")
	_, err := c.Health(ctx)

	if err == nil {
		t.Fatal("expected error from closed client, got nil")
	}
	if !errors.Is(err, openshell.ErrUnavailable) {
		t.Errorf("expected error to wrap openshell.ErrUnavailable, got: %v", err)
	}
}

func TestProviders(t *testing.T) {
	tests := []struct {
		name           string
		providers      []*types.Provider
		expectLen      int
		expectProvider *types.Provider // if set, check first provider matches
	}{
		{
			name: "with provider",
			providers: []*types.Provider{
				{Name: "p1", Type: "openai"},
			},
			expectLen: 1,
			expectProvider: &types.Provider{
				Name: "p1",
				Type: "openai",
			},
		},
		{
			name:           "no providers",
			providers:      []*types.Provider{},
			expectLen:      0,
			expectProvider: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fc := fake.NewClient()
			for _, p := range tt.providers {
				fc.AddProvider("default", p)
			}
			c := NewFromClient(fc, "default")

			providers, err := c.Providers(ctx)
			if err != nil {
				t.Fatalf("Providers() returned unexpected error: %v", err)
			}
			if len(providers) != tt.expectLen {
				t.Errorf("expected %d providers, got %d", tt.expectLen, len(providers))
			}
			if tt.expectProvider != nil && len(providers) > 0 {
				if providers[0].Name != tt.expectProvider.Name {
					t.Errorf("expected first provider Name=%q, got %q", tt.expectProvider.Name, providers[0].Name)
				}
				if providers[0].Type != tt.expectProvider.Type {
					t.Errorf("expected first provider Type=%q, got %q", tt.expectProvider.Type, providers[0].Type)
				}
			}
		})
	}
}

// TestNewFromClientDefaultsWorkspace pins the single-owner workspace default:
// an empty workspace binds the client to "default". A provider registered under
// "default" is visible through a client constructed with "" — proving the
// default lives here and nowhere else (New passes t.Workspace straight through).
func TestNewFromClientDefaultsWorkspace(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddProvider("default", &types.Provider{Name: "p1", Type: "openai"})

	c := NewFromClient(fc, "")
	providers, err := c.Providers(ctx)
	if err != nil {
		t.Fatalf("Providers() returned unexpected error: %v", err)
	}
	if len(providers) != 1 || providers[0].Name != "p1" {
		t.Errorf("empty workspace should bind to %q; got providers %+v", defaultWorkspace, providers)
	}
}

func TestProvidersErrorTranslated(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.Close()

	c := NewFromClient(fc, "default")
	_, err := c.Providers(ctx)

	if err == nil {
		t.Fatal("expected error from closed client, got nil")
	}
	if !errors.Is(err, openshell.ErrUnavailable) {
		t.Errorf("expected error to wrap openshell.ErrUnavailable, got: %v", err)
	}
}

func TestTranslate(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		expectSent error // expected sentinel, or nil
	}{
		{
			name:       "nil error",
			err:        nil,
			expectSent: nil,
		},
		{
			name: "not found",
			err: &types.StatusError{
				Code:    types.ErrorNotFound,
				Message: "not found",
			},
			expectSent: openshell.ErrNotFound,
		},
		{
			name: "unavailable",
			err: &types.StatusError{
				Code:    types.ErrorUnavailable,
				Message: "unavailable",
			},
			expectSent: openshell.ErrUnavailable,
		},
		{
			name: "unimplemented",
			err: &types.StatusError{
				Code:    types.ErrorUnimplemented,
				Message: "unimplemented",
			},
			expectSent: openshell.ErrUnsupported,
		},
		{
			name: "unauthenticated",
			err: &types.StatusError{
				Code:    types.ErrorUnauthenticated,
				Message: "unauthenticated",
			},
			expectSent: openshell.ErrUnauthenticated,
		},
		{
			name: "permission denied",
			err: &types.StatusError{
				Code:    types.ErrorPermissionDenied,
				Message: "permission denied",
			},
			expectSent: openshell.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translate(tt.err)
			if tt.expectSent == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				if !errors.Is(result, tt.expectSent) {
					t.Errorf("expected error to wrap %v, got %v", tt.expectSent, result)
				}
			}
		})
	}

	// Test pass-through of non-SDK error
	nonSDKErr := errors.New("boom")
	result := translate(nonSDKErr)
	if result != nonSDKErr {
		t.Errorf("expected pass-through of non-SDK error, got %v", result)
	}
	if errors.Is(result, openshell.ErrNotFound) || errors.Is(result, openshell.ErrUnavailable) {
		t.Errorf("non-SDK error should not match any sentinel")
	}
}

func TestCloseIdempotent(t *testing.T) {
	c := NewFromClient(fake.NewClient(), "default")

	err1 := c.Close()
	if err1 != nil {
		t.Errorf("first Close() returned unexpected error: %v", err1)
	}

	err2 := c.Close()
	if err2 != nil {
		t.Errorf("second Close() returned unexpected error: %v", err2)
	}
}
