package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// routeNamePattern is a format-only guard for inference route names: a
// DNS-label-ish token, optionally dotted (e.g. "inference.local"). It rejects
// empty/whitespace/leading-or-trailing-punctuation garbage at load time; it is
// NOT an allowlist — the gateway stays the authority on which names exist and
// returns ErrInvalidArgument for unknown ones at apply.
var routeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// expandStrict interpolates ${VAR} references in raw using getenv. A referenced
// but unset variable is an error (strict — never os.ExpandEnv, which is lenient).
// A $$ sequence and a bare $ not followed by { are non-special and left as-is.
//
// Because getenv is func(string) string, an unset variable and one set to the
// empty string are indistinguishable; both are treated as unset (an error).
// A literal empty value must therefore be written empty in YAML, not as ${VAR}.
//
// Resolve is the production entry point (it calls expand directly and aggregates
// missing-variable errors with field paths); expandStrict is the single-string
// wrapper exercised by the scanner's unit tests.
func expandStrict(raw string, getenv func(string) string) (string, error) {
	out, missing := expand(raw, getenv)
	if len(missing) > 0 {
		return "", fmt.Errorf("unresolved variables %v", missing)
	}
	return out, nil
}

// expand is the scanner shared by Expand and Resolve. It returns the expanded
// string and the names of any referenced-but-unset variables.
func expand(raw string, getenv func(string) string) (string, []string) {
	var out strings.Builder
	var missing []string

	for i := 0; i < len(raw); i++ {
		if raw[i] != '$' {
			out.WriteByte(raw[i])
			continue
		}
		switch {
		case i+1 < len(raw) && raw[i+1] == '$':
			out.WriteString("$$") // $$ is non-special, left as-is
			i++
		case i+1 < len(raw) && raw[i+1] == '{':
			end := strings.IndexByte(raw[i+2:], '}')
			if end == -1 {
				out.WriteByte('$') // no closing brace: literal $
				continue
			}
			name := raw[i+2 : i+2+end]
			if val := getenv(name); val != "" {
				out.WriteString(val)
			} else {
				missing = append(missing, name)
			}
			i += 2 + end
		default:
			out.WriteByte('$') // bare $
		}
	}
	return out.String(), missing
}

// destinationHasTraversal reports whether a sandbox destination path contains a
// ".." segment — the one traversal vector with no legitimate use in a
// destination. Absolute paths are deliberately allowed: sandbox destinations are
// conventionally absolute (e.g. "/sandbox/review.md"), and the openshell runtime
// remains the authority on where an upload actually lands.
func destinationHasTraversal(p string) bool {
	return slices.Contains(strings.Split(p, "/"), "..")
}

// Resolve returns a copy of h with every string field interpolated via Expand.
// Missing variables across all fields are aggregated into one error that names
// each variable and its field path.
func Resolve(h *Harness, getenv func(string) string) (*Harness, error) {
	resolved := *h // shallow copy; slices/maps holding expanded values are reallocated below
	var errs []string

	// exp expands a single field, recording any missing variables against path.
	exp := func(path, val string) string {
		out, missing := expand(val, getenv)
		for _, name := range missing {
			errs = append(errs, fmt.Sprintf("unresolved variable ${%s} (%s)", name, path))
		}
		return out
	}

	s := &resolved.Spec
	s.Target.Gateway = exp("target.gateway", h.Spec.Target.Gateway)
	s.Target.Workspace = exp("target.workspace", h.Spec.Target.Workspace)

	if r := h.Spec.Target.Registration; r != nil {
		reg := *r
		reg.Endpoint = exp("target.registration.endpoint", r.Endpoint)
		if r.OIDC != nil {
			o := *r.OIDC
			o.Issuer = exp("target.registration.oidc.issuer", r.OIDC.Issuer)
			o.ClientID = exp("target.registration.oidc.clientId", r.OIDC.ClientID)
			o.Audience = exp("target.registration.oidc.audience", r.OIDC.Audience)
			reg.OIDC = &o
		}
		s.Target.Registration = &reg
		if reg.Endpoint == "" {
			errs = append(errs, "target.registration.endpoint: required")
		}
		if reg.OIDC == nil {
			errs = append(errs, "target.registration.oidc: required")
		} else {
			if reg.OIDC.Issuer == "" {
				errs = append(errs, "target.registration.oidc.issuer: required")
			}
			if reg.OIDC.ClientID == "" {
				errs = append(errs, "target.registration.oidc.clientId: required")
			}
			if reg.OIDC.Audience == "" {
				errs = append(errs, "target.registration.oidc.audience: required")
			}
		}
	}

	s.Inference.Route = exp("inference.route", h.Spec.Inference.Route)
	// Format-only check: reject a malformed route name at load time; the gateway
	// remains the authority on which names actually exist (no allowlist here).
	if s.Inference.Route != "" && !routeNamePattern.MatchString(s.Inference.Route) {
		errs = append(errs, fmt.Sprintf("inference.route: %q is malformed (want a DNS-label-like name such as \"inference.local\")", s.Inference.Route))
	}
	s.Inference.Provider = exp("inference.provider", h.Spec.Inference.Provider)
	s.Inference.Model = exp("inference.model", h.Spec.Inference.Model)
	s.Inference.Timeout = exp("inference.timeout", h.Spec.Inference.Timeout)
	// Validate the (now expanded) timeout once, here at resolve time, so the plan
	// diff and reconcile write can parse it without handling an error.
	if _, err := s.Inference.TimeoutSecs(); err != nil {
		errs = append(errs, fmt.Sprintf("inference.timeout: %v", err))
	}
	// A configured inference block must name both a provider and a model: the
	// gateway rejects a route write that lacks either, and reconcile has nothing
	// to write without them. Catch it here at resolve time with a clear message
	// instead of surfacing a late ErrInvalidArgument on apply. Route/timeout alone
	// (or verify alone) don't identify a route to reconcile. Kept in step with
	// plan.isInferenceConfigured.
	if s.Inference.Route != "" || s.Inference.Provider != "" || s.Inference.Model != "" || s.Inference.Timeout != "" {
		if s.Inference.Provider == "" {
			errs = append(errs, "inference.provider: required when inference is configured")
		}
		if s.Inference.Model == "" {
			errs = append(errs, "inference.model: required when inference is configured")
		}
	}
	if v := h.Spec.Inference.Verify; v != nil {
		b := *v // copy so the resolved struct never aliases the input's *bool
		s.Inference.Verify = &b
	}

	s.Sandbox.Image = exp("sandbox.image", h.Spec.Sandbox.Image)
	if p := h.Spec.Sandbox.Policy; p != nil {
		np := *p // copy so the resolved struct never aliases the input's PolicyRef
		np.File = exp("sandbox.policy.file", p.File)
		s.Sandbox.Policy = &np
	}
	if len(h.Spec.Sandbox.Providers) > 0 {
		s.Sandbox.Providers = make([]string, len(h.Spec.Sandbox.Providers))
		for i, p := range h.Spec.Sandbox.Providers {
			path := fmt.Sprintf("sandbox.providers[%d]", i)
			name := exp(path, p)
			s.Sandbox.Providers[i] = name
			if name == "" {
				errs = append(errs, path+": required")
			}
		}
	}
	if len(h.Spec.Sandbox.Env) > 0 {
		s.Sandbox.Env = make(map[string]string, len(h.Spec.Sandbox.Env))
		for k, v := range h.Spec.Sandbox.Env {
			s.Sandbox.Env[k] = exp("sandbox.env."+k, v)
		}
	}

	s.Agent.Type = exp("agent.type", h.Spec.Agent.Type)
	if len(h.Spec.Agent.Args) > 0 {
		s.Agent.Args = make([]string, len(h.Spec.Agent.Args))
		for i, a := range h.Spec.Agent.Args {
			s.Agent.Args[i] = exp(fmt.Sprintf("agent.args[%d]", i), a)
		}
	}

	s.Source.Repo = exp("source.repo", h.Spec.Source.Repo)
	s.Source.Ref = exp("source.ref", h.Spec.Source.Ref)
	s.Source.Destination = exp("source.destination", h.Spec.Source.Destination)
	if destinationHasTraversal(s.Source.Destination) {
		errs = append(errs, `source.destination: must not contain a ".." path segment`)
	}
	s.Source.Submodules = exp("source.submodules", h.Spec.Source.Submodules)

	if len(h.Spec.Payloads) > 0 {
		s.Payloads = make([]Payload, len(h.Spec.Payloads))
		for i, p := range h.Spec.Payloads {
			base := fmt.Sprintf("payloads[%d]", i)
			dest := exp(base+".destination", p.Destination)
			if destinationHasTraversal(dest) {
				errs = append(errs, base+`.destination: must not contain a ".." path segment`)
			}
			s.Payloads[i] = Payload{
				Source:      exp(base+".source", p.Source),
				Content:     exp(base+".content", p.Content),
				Destination: dest,
			}
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return &resolved, nil
}
