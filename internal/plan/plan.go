// Package plan provides typed diff models and a pure builder for harness config reconciliation.
//
// Build diffs desired config against the current gateway state and produces a
// typed Plan. All I/O is isolated in ReadCurrentState; Build itself is pure. The
// JSON, YAML, and table renderings are three views of the same Plan model.
package plan

import (
	"strings"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
)

// Action names the operation the plan would perform.
type Action string

const (
	ActionNoop             Action = "noop"
	ActionCreate           Action = "create"
	ActionUpdate           Action = "update"
	ActionValidate         Action = "validate"
	ActionLoginRequired    Action = "login-required"
	ActionAdoptionRequired Action = "adoption-required"
	ActionCreateSandbox    Action = "create-sandbox"
	ActionUpload           Action = "upload"
	ActionExecute          Action = "execute"
	ActionDeleteSandbox    Action = "delete-sandbox"
)

// Section names a group of plan resources.
type Section string

const (
	SectionTarget    Section = "target"
	SectionProviders Section = "providers"
	SectionInference Section = "inference"
	SectionRun       Section = "run"
)

// Resource represents a single reconciliation unit.
type Resource struct {
	Name   string `json:"name" yaml:"name"`
	Action Action `json:"action" yaml:"action"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"` // redaction-safe
}

// Group clusters resources by section.
type Group struct {
	Section   Section    `json:"section" yaml:"section"`
	Resources []Resource `json:"resources" yaml:"resources"`
}

// Plan is the typed output of diffing desired config against current gateway state.
type Plan struct {
	Target openshell.Target `json:"target" yaml:"target"`
	Groups []Group          `json:"groups" yaml:"groups"`
}

// Build produces a Plan from desired config and current state. Build is pure: no I/O,
// no client, no context. All state information flows through the CurrentState argument.
func Build(desired *config.Harness, current CurrentState) *Plan {
	p := &Plan{
		Target: openshell.Target{
			Gateway:   desired.Spec.Target.Gateway,
			Workspace: desired.Spec.Target.Workspace,
		},
	}

	// TARGET group: always emitted, one resource.
	p.Groups = append(p.Groups, buildTargetGroup(desired, current))

	// PROVIDERS group: emitted only if desired has providers.
	if len(desired.Spec.Providers) > 0 {
		p.Groups = append(p.Groups, buildProvidersGroup(desired, current))
	}

	// INFERENCE group: emitted only if desired.Spec.Inference is non-empty.
	if isInferenceConfigured(desired.Spec.Inference) {
		p.Groups = append(p.Groups, buildInferenceGroup(desired, current))
	}

	// RUN group: emitted only if any run-related config is present.
	if hasRunConfig(desired) {
		p.Groups = append(p.Groups, buildRunGroup(desired))
	}

	return p
}

// buildTargetGroup returns the TARGET group: validate when the gateway is
// reachable, otherwise login-required.
func buildTargetGroup(desired *config.Harness, current CurrentState) Group {
	gatewayName := desired.Spec.Target.Gateway

	var action Action
	var detail string

	if current.Reachable {
		action = ActionValidate
		detail = "gateway " + gatewayName
		if current.Health.Version != "" {
			detail += " v" + current.Health.Version
		}
	} else {
		action = ActionLoginRequired
		detail = "gateway unreachable or unauthenticated"
	}

	return Group{
		Section: SectionTarget,
		Resources: []Resource{
			{
				Name:   gatewayName,
				Action: action,
				Detail: detail,
			},
		},
	}
}

// buildProvidersGroup returns the PROVIDERS group.
// Matches desired providers by name against current.Providers.
func buildProvidersGroup(desired *config.Harness, current CurrentState) Group {
	group := Group{Section: SectionProviders}

	// Build a map of current providers by name for lookup.
	currentByName := make(map[string]openshell.Provider)
	for _, p := range current.Providers {
		currentByName[p.Name] = p
	}

	for _, desiredProv := range desired.Spec.Providers {
		var action Action
		var detail string

		currentProv, exists := currentByName[desiredProv.Name]

		if !exists {
			// Provider not present in current state.
			if desiredProv.Management == "managed" {
				action = ActionCreate
			} else {
				// referenced or unknown management
				action = ActionAdoptionRequired
			}
		} else if desiredProv.Type != "" && currentProv.Type != desiredProv.Type {
			// Type mismatch.
			action = ActionUpdate
		} else {
			// Provider exists and type matches (or desired type is empty).
			action = ActionNoop
		}

		// Build detail string: type + management + credentials source if applicable.
		detail = buildProviderDetail(&desiredProv)

		group.Resources = append(group.Resources, Resource{
			Name:   desiredProv.Name,
			Action: action,
			Detail: detail,
		})
	}

	return group
}

// buildProviderDetail constructs a redaction-safe detail string for a provider.
func buildProviderDetail(prov *config.Provider) string {
	detail := prov.Type
	if detail == "" {
		detail = "(type unspecified)"
	}
	detail += "; management: " + prov.Management

	if prov.Credentials != nil {
		detail += "; credential source: " + prov.Credentials.Describe()
	}

	return detail
}

// buildInferenceGroup returns the INFERENCE group. The action is always
// validate: the gateway does not report inference state, so the plan can only
// validate the configured route against the desired config.
func buildInferenceGroup(desired *config.Harness, current CurrentState) Group {
	group := Group{Section: SectionInference}

	detail := buildInferenceDetail(desired.Spec.Inference)

	group.Resources = append(group.Resources, Resource{
		Name:   "inference",
		Action: ActionValidate,
		Detail: detail,
	})

	return group
}

// buildInferenceDetail constructs a detail string for inference config.
func buildInferenceDetail(inf config.Inference) string {
	detail := ""

	if inf.Provider != "" {
		detail += inf.Provider
	}
	if inf.Model != "" {
		if detail != "" {
			detail += "/"
		}
		detail += inf.Model
	}

	if detail == "" {
		detail = "(provider/model unspecified)"
	}

	detail += "; config only (gateway does not report inference state)"

	return detail
}

// buildRunGroup returns the RUN group with descriptive actions.
func buildRunGroup(desired *config.Harness) Group {
	group := Group{Section: SectionRun}

	// Create sandbox action.
	if desired.Spec.Sandbox.Image != "" {
		detail := ""
		if len(desired.Spec.Sandbox.Providers) > 0 {
			detail = "sandbox providers: " + strings.Join(desired.Spec.Sandbox.Providers, ", ")
		}
		group.Resources = append(group.Resources, Resource{
			Name:   desired.Spec.Sandbox.Image,
			Action: ActionCreateSandbox,
			Detail: detail,
		})
	}

	// Upload actions per source repo.
	if desired.Spec.Source.Repo != "" {
		group.Resources = append(group.Resources, Resource{
			Name:   desired.Spec.Source.Repo,
			Action: ActionUpload,
			Detail: "source repo",
		})
	}

	// Upload actions per payload.
	for _, payload := range desired.Spec.Payloads {
		name := payload.Source
		if name == "" && payload.Content != "" {
			name = payload.Destination
		}
		if name != "" {
			group.Resources = append(group.Resources, Resource{
				Name:   name,
				Action: ActionUpload,
				Detail: "payload",
			})
		}
	}

	// Execute action.
	if desired.Spec.Agent.Type != "" {
		detail := ""
		if len(desired.Spec.Agent.Args) > 0 {
			detail = "args: " + strings.Join(desired.Spec.Agent.Args, ", ")
		}
		group.Resources = append(group.Resources, Resource{
			Name:   desired.Spec.Agent.Type,
			Action: ActionExecute,
			Detail: detail,
		})
	}

	// Delete sandbox action (only if keep is false).
	if desired.Spec.Sandbox.Image != "" && !desired.Spec.Sandbox.Keep {
		group.Resources = append(group.Resources, Resource{
			Name:   desired.Spec.Sandbox.Image,
			Action: ActionDeleteSandbox,
			Detail: "",
		})
	}

	return group
}

// isInferenceConfigured returns true if inference config has any meaningful fields set.
func isInferenceConfigured(inf config.Inference) bool {
	return inf.Route != "" || inf.Provider != "" || inf.Model != "" || inf.Timeout != "" || inf.Verify
}

// hasRunConfig returns true if any run-related config is present.
func hasRunConfig(desired *config.Harness) bool {
	return desired.Spec.Sandbox.Image != "" ||
		desired.Spec.Source.Repo != "" ||
		len(desired.Spec.Payloads) > 0 ||
		desired.Spec.Agent.Type != ""
}

// TableSection represents a section ready for table rendering.
type TableSection struct {
	Title   string
	Headers []string
	Rows    [][]string
}

// TableSections projects the Plan into table format.
// One TableSection per group; headers are {"ACTION", "NAME", "DETAIL"}.
func (p *Plan) TableSections() []TableSection {
	var sections []TableSection

	for _, group := range p.Groups {
		section := TableSection{
			Title:   string(group.Section),
			Headers: []string{"ACTION", "NAME", "DETAIL"},
		}

		for _, res := range group.Resources {
			section.Rows = append(section.Rows, []string{
				string(res.Action),
				res.Name,
				res.Detail,
			})
		}

		sections = append(sections, section)
	}

	return sections
}
