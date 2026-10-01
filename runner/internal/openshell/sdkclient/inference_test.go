package sdkclient

import (
	"context"
	"errors"
	"testing"

	fake "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1/fake"

	"github.com/stackrox/harness-openshell/runner/internal/openshell"
)

func TestInferenceRoutesUnsupportedInOpenShellV012(t *testing.T) {
	c := NewFromClient(fake.NewClient(), "default")
	ctx := context.Background()

	if _, err := c.GetInferenceRoute(ctx, "inference.local"); !errors.Is(err, openshell.ErrUnsupported) {
		t.Fatalf("GetInferenceRoute: want ErrUnsupported, got %v", err)
	}
	if _, err := c.SetInferenceRoute(ctx, openshell.InferenceRouteConfig{
		Provider: "vertex", Model: "gemini-2.5-pro",
	}); !errors.Is(err, openshell.ErrUnsupported) {
		t.Fatalf("SetInferenceRoute: want ErrUnsupported, got %v", err)
	}
	if err := c.DeleteInferenceRoute(ctx, "inference.local"); !errors.Is(err, openshell.ErrUnsupported) {
		t.Fatalf("DeleteInferenceRoute: want ErrUnsupported, got %v", err)
	}
}
