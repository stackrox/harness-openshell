package sdkclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"
	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"
	"github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/types"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestFromSDKProviderExposesIdentityOnly(t *testing.T) {
	got := fromSDKProvider(&v1.Provider{
		Name:   "github",
		Type:   "github",
		Labels: map[string]string{"owner": "test"},
		Spec: types.ProviderSpec{
			Config:      map[string]string{"endpoint": "https://example.invalid"},
			Credentials: map[string]string{"token": "must-not-cross-boundary"},
		},
	})
	if got.Name != "github" || got.Type != "github" {
		t.Fatalf("identity = %#v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "must-not-cross-boundary") || strings.Contains(string(data), "example.invalid") {
		t.Fatal("provider credentials or configuration crossed the read boundary")
	}
}

func TestGetProvider(t *testing.T) {
	ctx := context.Background()
	fc := fake.NewClient()
	fc.AddProvider("default", &types.Provider{Name: "github", Type: "github"})
	c := NewFromClient(fc, "default")
	defer c.Close()
	got, err := c.GetProvider(ctx, "github")
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.Name != "github" || got.Type != "github" {
		t.Errorf("unexpected provider: %+v", got)
	}
	if _, err := c.GetProvider(ctx, "absent"); !errors.Is(err, openshell.ErrNotFound) {
		t.Errorf("GetProvider(absent): want ErrNotFound, got %v", err)
	}
}
