package plan

import (
	"strings"
	"testing"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

func TestBuild_TargetValidateWhenReachable(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health: openshell.Health{
			Healthy: true,
			Version: "0.0.110",
		},
	}

	plan := Build(desired, current)

	if len(plan.Groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(plan.Groups))
	}
	if plan.Groups[0].Section != SectionTarget {
		t.Errorf("expected TARGET section, got %s", plan.Groups[0].Section)
	}
	if len(plan.Groups[0].Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(plan.Groups[0].Resources))
	}

	res := plan.Groups[0].Resources[0]
	if res.Action != ActionValidate {
		t.Errorf("expected ActionValidate, got %s", res.Action)
	}
	if res.Name != "test-gateway" {
		t.Errorf("expected name 'test-gateway', got %s", res.Name)
	}
	if res.Detail != "gateway test-gateway v0.0.110" {
		t.Errorf("unexpected detail: %s", res.Detail)
	}
}

func TestBuild_TargetLoginRequiredWhenUnreachable(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: false,
	}

	plan := Build(desired, current)

	res := plan.Groups[0].Resources[0]
	if res.Action != ActionLoginRequired {
		t.Errorf("expected ActionLoginRequired, got %s", res.Action)
	}
	if res.Detail != "gateway unreachable or unauthenticated" {
		t.Errorf("unexpected detail: %s", res.Detail)
	}
}

func TestBuild_ProviderPresentNoop(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{
					Name:       "github",
					Type:       "github",
					Management: "managed",
				},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{
			{Name: "github", Type: "github"},
		},
	}

	plan := Build(desired, current)

	var provGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionProviders {
			provGroup = &plan.Groups[i]
			break
		}
	}

	if provGroup == nil {
		t.Fatal("expected PROVIDERS group")
	}
	if len(provGroup.Resources) != 1 {
		t.Errorf("expected 1 provider resource, got %d", len(provGroup.Resources))
	}

	res := provGroup.Resources[0]
	if res.Action != ActionNoop {
		t.Errorf("expected ActionNoop, got %s", res.Action)
	}
	if res.Name != "github" {
		t.Errorf("expected name 'github', got %s", res.Name)
	}
}

func TestBuild_ProviderAbsentManaged(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{
					Name:       "gcp",
					Type:       "google-vertex-ai",
					Management: "managed",
				},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{},
	}

	plan := Build(desired, current)

	var provGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionProviders {
			provGroup = &plan.Groups[i]
			break
		}
	}

	res := provGroup.Resources[0]
	if res.Action != ActionCreate {
		t.Errorf("expected ActionCreate, got %s", res.Action)
	}
}

func TestBuild_ProviderAbsentReferenced(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{
					Name:       "external",
					Management: "referenced",
				},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{},
	}

	plan := Build(desired, current)

	var provGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionProviders {
			provGroup = &plan.Groups[i]
			break
		}
	}

	res := provGroup.Resources[0]
	if res.Action != ActionAdoptionRequired {
		t.Errorf("expected ActionAdoptionRequired, got %s", res.Action)
	}
}

func TestBuild_ProviderTypeUpdate(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{
					Name:       "github",
					Type:       "github-new",
					Management: "managed",
				},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{
			{Name: "github", Type: "github-old"},
		},
	}

	plan := Build(desired, current)

	var provGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionProviders {
			provGroup = &plan.Groups[i]
			break
		}
	}

	res := provGroup.Resources[0]
	if res.Action != ActionUpdate {
		t.Errorf("expected ActionUpdate, got %s", res.Action)
	}
}

func TestBuild_ProviderDetailIncludesCredentials(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{
					Name:       "gcp",
					Type:       "google-vertex-ai",
					Management: "managed",
					Credentials: &config.SecretRef{
						Source: "gcloud-adc",
					},
				},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{},
	}

	plan := Build(desired, current)

	var provGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionProviders {
			provGroup = &plan.Groups[i]
			break
		}
	}

	detail := provGroup.Resources[0].Detail
	if !strings.Contains(detail, "credential source:") {
		t.Errorf("expected credential source in detail: %s", detail)
	}
	if !strings.Contains(detail, "gcloud ADC") {
		t.Errorf("expected 'gcloud ADC' in detail: %s", detail)
	}
}

func TestBuild_InferenceGroupWhenConfigured(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Inference: config.Inference{
				Provider: "my-gcp",
				Model:    "claude-haiku-4-5",
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	var infGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionInference {
			infGroup = &plan.Groups[i]
			break
		}
	}

	if infGroup == nil {
		t.Fatal("expected INFERENCE group")
	}
	if len(infGroup.Resources) != 1 {
		t.Errorf("expected 1 inference resource, got %d", len(infGroup.Resources))
	}

	res := infGroup.Resources[0]
	if res.Action != ActionValidate {
		t.Errorf("expected ActionValidate, got %s", res.Action)
	}
	if !strings.Contains(res.Detail, "my-gcp/claude-haiku-4-5") {
		t.Errorf("expected provider/model in detail: %s", res.Detail)
	}
	if !strings.Contains(res.Detail, "gateway does not report inference state") {
		t.Errorf("expected config-only note in detail: %s", res.Detail)
	}
}

func TestInferenceAction(t *testing.T) {
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8", Timeout: "60s"}

	tests := []struct {
		name string
		cur  InferenceState
		want Action
	}{
		{
			name: "not capable falls back to validate",
			cur:  InferenceState{Capable: false},
			want: ActionValidate,
		},
		{
			name: "capable but absent creates",
			cur:  InferenceState{Capable: true, Present: false},
			want: ActionCreate,
		},
		{
			name: "model mismatch updates",
			cur:  InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-sonnet-5", TimeoutSecs: 60},
			want: ActionUpdate,
		},
		{
			name: "provider mismatch updates",
			cur:  InferenceState{Capable: true, Present: true, Provider: "aws", Model: "claude-opus-4-8", TimeoutSecs: 60},
			want: ActionUpdate,
		},
		{
			name: "timeout mismatch updates",
			cur:  InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 30},
			want: ActionUpdate,
		},
		{
			name: "exact match noops",
			cur:  InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 60},
			want: ActionNoop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferenceAction(desired, tt.cur); got != tt.want {
				t.Errorf("InferenceAction = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestInferenceAction_UnsetTimeoutMatchesZero pins that an unset desired timeout
// (secs 0, gateway default) noops against a route the gateway reports as 0.
func TestInferenceAction_UnsetTimeoutMatchesZero(t *testing.T) {
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8"} // no timeout
	cur := InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 0}
	if got := InferenceAction(desired, cur); got != ActionNoop {
		t.Errorf("InferenceAction = %s, want noop", got)
	}
}

// TestInferenceAction_UnsetTimeoutIgnoresGatewayDefault is the finding-1
// regression: an unset desired timeout means "don't care", so it must noop even
// when the gateway reports a nonzero default it applied — otherwise the plan
// reports a perpetual update it can never resolve.
func TestInferenceAction_UnsetTimeoutIgnoresGatewayDefault(t *testing.T) {
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8"} // no timeout
	cur := InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 300}
	if got := InferenceAction(desired, cur); got != ActionNoop {
		t.Errorf("InferenceAction = %s, want noop (unset timeout must not chase the gateway default)", got)
	}
}

// TestInferenceAction_ZeroSecondsTimeoutIsDontCare pins the whole-spec finding:
// "0s" resolves to 0 seconds, which the gateway can never store (0 => default),
// so it must be treated as "don't care" just like "" — otherwise it churns an
// update forever against the gateway's nonzero default.
func TestInferenceAction_ZeroSecondsTimeoutIsDontCare(t *testing.T) {
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8", Timeout: "0s"}
	cur := InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 60}
	if got := InferenceAction(desired, cur); got != ActionNoop {
		t.Errorf("InferenceAction = %s, want noop (\"0s\" must not chase the gateway default)", got)
	}
}

// TestInferenceAction_ExplicitTimeoutStillDiffs guards that the finding-1 fix did
// not neuter the timeout diff: an explicitly configured timeout still updates
// against a mismatched gateway value.
func TestInferenceAction_ExplicitTimeoutStillDiffs(t *testing.T) {
	desired := config.Inference{Provider: "gcp", Model: "claude-opus-4-8", Timeout: "60s"}
	cur := InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8", TimeoutSecs: 300}
	if got := InferenceAction(desired, cur); got != ActionUpdate {
		t.Errorf("InferenceAction = %s, want update (explicit timeout must still diff)", got)
	}
}

func TestResolveInferenceRoute(t *testing.T) {
	if got := ResolveInferenceRoute(""); got != DefaultInferenceRoute {
		t.Errorf("empty route: got %q, want %q", got, DefaultInferenceRoute)
	}
	if got := ResolveInferenceRoute("custom-route"); got != "custom-route" {
		t.Errorf("explicit route: got %q, want %q", got, "custom-route")
	}
}

func TestBuild_InferenceRealDiff(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target:    config.Target{Gateway: "test-gateway"},
			Inference: config.Inference{Provider: "gcp", Model: "claude-opus-4-8"},
		},
	}

	infGroup := func(p *Plan) *Resource {
		for i := range p.Groups {
			if p.Groups[i].Section == SectionInference {
				return &p.Groups[i].Resources[0]
			}
		}
		return nil
	}

	// Capable + absent → create.
	res := infGroup(Build(desired, CurrentState{
		Reachable: true,
		Inference: InferenceState{Capable: true, Present: false},
	}))
	if res == nil || res.Action != ActionCreate {
		t.Fatalf("absent route: want create, got %+v", res)
	}
	if strings.Contains(res.Detail, "config only") {
		t.Errorf("create detail should not carry the config-only caveat: %s", res.Detail)
	}

	// Capable + matching → noop.
	res = infGroup(Build(desired, CurrentState{
		Reachable: true,
		Inference: InferenceState{Capable: true, Present: true, Provider: "gcp", Model: "claude-opus-4-8"},
	}))
	if res == nil || res.Action != ActionNoop {
		t.Fatalf("matching route: want noop, got %+v", res)
	}
	if !strings.Contains(res.Detail, "matches gateway") {
		t.Errorf("noop detail should say it matches: %s", res.Detail)
	}
}

func TestBuild_NoInferenceGroupWhenEmpty(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	for _, group := range plan.Groups {
		if group.Section == SectionInference {
			t.Fatal("expected no INFERENCE group when inference config is empty")
		}
	}
}

func TestBuild_RunGroupWithSandbox(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Sandbox: config.Sandbox{
				Image:     "quay.io/test:latest",
				Providers: []string{"github", "gcp"},
				Keep:      false,
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	var runGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionRun {
			runGroup = &plan.Groups[i]
			break
		}
	}

	if runGroup == nil {
		t.Fatal("expected RUN group")
	}

	// Should have create-sandbox and delete-sandbox
	hasCreateSandbox := false
	hasDeleteSandbox := false
	for _, res := range runGroup.Resources {
		if res.Action == ActionCreateSandbox {
			hasCreateSandbox = true
			if !strings.Contains(res.Detail, "github") {
				t.Errorf("expected provider list in detail: %s", res.Detail)
			}
		}
		if res.Action == ActionDeleteSandbox {
			hasDeleteSandbox = true
		}
	}

	if !hasCreateSandbox {
		t.Error("expected ActionCreateSandbox in RUN group")
	}
	if !hasDeleteSandbox {
		t.Error("expected ActionDeleteSandbox in RUN group (keep=false)")
	}
}

func TestBuild_RunGroupWithPayloads(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Source: config.Source{
				Repo: "https://github.com/example/repo",
			},
			Payloads: []config.Payload{
				{Source: ".agents/skills", Destination: "/sandbox/.agents/skills"},
				{Content: "# Config", Destination: "/sandbox/CONFIG.md"},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	var runGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionRun {
			runGroup = &plan.Groups[i]
			break
		}
	}

	if runGroup == nil {
		t.Fatal("expected RUN group")
	}

	// Count uploads: 1 for source repo + 2 for payloads
	uploadCount := 0
	for _, res := range runGroup.Resources {
		if res.Action == ActionUpload {
			uploadCount++
		}
	}

	if uploadCount != 3 {
		t.Errorf("expected 3 upload actions, got %d", uploadCount)
	}
}

func TestBuild_RunGroupWithAgent(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Agent: config.Agent{
				Type: "claude",
				Args: []string{"--bare", "--model=opus"},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	var runGroup *Group
	for i := range plan.Groups {
		if plan.Groups[i].Section == SectionRun {
			runGroup = &plan.Groups[i]
			break
		}
	}

	if runGroup == nil {
		t.Fatal("expected RUN group")
	}

	hasExecute := false
	for _, res := range runGroup.Resources {
		if res.Action == ActionExecute {
			hasExecute = true
			if res.Name != "claude" {
				t.Errorf("expected agent type 'claude', got %s", res.Name)
			}
			if !strings.Contains(res.Detail, "--bare") {
				t.Errorf("expected --bare in detail: %s", res.Detail)
			}
		}
	}

	if !hasExecute {
		t.Error("expected ActionExecute in RUN group")
	}
}

func TestBuild_NoRunGroupWhenEmpty(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
	}

	plan := Build(desired, current)

	for _, group := range plan.Groups {
		if group.Section == SectionRun {
			t.Fatal("expected no RUN group when run config is empty")
		}
	}
}

func TestPlan_TableSections(t *testing.T) {
	desired := &config.Harness{
		Spec: config.Spec{
			Target: config.Target{Gateway: "test-gateway"},
			Providers: []config.Provider{
				{Name: "github", Type: "github", Management: "managed"},
			},
		},
	}
	current := CurrentState{
		Reachable: true,
		Health:    openshell.Health{Healthy: true, Version: "0.0.110"},
		Providers: []openshell.Provider{},
	}

	plan := Build(desired, current)
	sections := plan.TableSections()

	if len(sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(sections))
	}

	// Check headers
	for _, section := range sections {
		if len(section.Headers) != 3 {
			t.Errorf("expected 3 headers, got %d: %v", len(section.Headers), section.Headers)
		}
		if section.Headers[0] != "ACTION" || section.Headers[1] != "NAME" || section.Headers[2] != "DETAIL" {
			t.Errorf("unexpected headers: %v", section.Headers)
		}
	}

	// Check section order
	if sections[0].Title != "target" {
		t.Errorf("expected first section 'target', got %s", sections[0].Title)
	}
	if sections[1].Title != "providers" {
		t.Errorf("expected second section 'providers', got %s", sections[1].Title)
	}

	// Check row format
	targetRows := sections[0].Rows
	if len(targetRows) != 1 {
		t.Errorf("expected 1 target row, got %d", len(targetRows))
	}
	if len(targetRows[0]) != 3 {
		t.Errorf("expected 3 columns in row, got %d", len(targetRows[0]))
	}
}
