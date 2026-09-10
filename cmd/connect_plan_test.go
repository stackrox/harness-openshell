package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

func TestConnectAndBuildPlanOfflineKeepsProvidersUninspected(t *testing.T) {
	workflow := &resolvedWorkflow{
		Desired: &config.Harness{Spec: config.Spec{
			Sandbox: config.Sandbox{Providers: []string{"github"}},
		}},
		Target: openshell.Target{},
	}
	var stderr strings.Builder
	client, planned, current, err := connectAndBuildPlan(
		context.Background(),
		func(context.Context, openshell.Target) (openshell.Client, error) {
			return nil, errors.New("gateway unavailable")
		},
		workflow,
		true,
		&stderr,
	)
	if err != nil {
		t.Fatalf("connectAndBuildPlan: %v", err)
	}
	if client != nil {
		t.Fatal("offline preview returned a client")
	}
	if current.Inspected {
		t.Fatal("offline preview marked gateway as inspected")
	}
	if got := planned.Groups[1].Resources[0].Action; got != plan.ActionNotInspected {
		t.Fatalf("provider action = %q, want %q", got, plan.ActionNotInspected)
	}
	if !strings.Contains(stderr.String(), "active gateway unreachable") {
		t.Fatalf("warning = %q, want active gateway context", stderr.String())
	}
}

func TestConnectAndBuildPlanNonDryRunRejectsConnectionFailure(t *testing.T) {
	workflow := &resolvedWorkflow{Desired: &config.Harness{}, Target: openshell.Target{Gateway: "ci"}}
	_, _, _, err := connectAndBuildPlan(
		context.Background(),
		func(context.Context, openshell.Target) (openshell.Client, error) {
			return nil, errors.New("gateway unavailable")
		},
		workflow,
		false,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), `connecting to gateway "ci"`) {
		t.Fatalf("error = %v, want connection context", err)
	}
}
