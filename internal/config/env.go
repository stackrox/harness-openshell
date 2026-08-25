package config

import (
	"fmt"
	"strings"
)

// Expand interpolates ${VAR} references in raw using getenv. A referenced but
// unset variable is an error (strict — never os.ExpandEnv, which is lenient).
// A $$ sequence and a bare $ not followed by { are non-special and left as-is.
//
// Because getenv is func(string) string, an unset variable and one set to the
// empty string are indistinguishable; both are treated as unset (an error).
// A literal empty value must therefore be written empty in YAML, not as ${VAR}.
func Expand(raw string, getenv func(string) string) (string, error) {
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

// Resolve returns a copy of h with every non-secret string field interpolated
// via Expand. Secret-bearing fields (Provider.Credentials) are copied through
// untouched — their values are never resolved, only their source is described.
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
	s.Target.Gateway = exp("spec.target.gateway", h.Spec.Target.Gateway)
	s.Target.Workspace = exp("spec.target.workspace", h.Spec.Target.Workspace)

	if r := h.Spec.Target.Registration; r != nil {
		reg := *r
		reg.Endpoint = exp("spec.target.registration.endpoint", r.Endpoint)
		if r.OIDC != nil {
			o := *r.OIDC
			o.Issuer = exp("spec.target.registration.oidc.issuer", r.OIDC.Issuer)
			o.ClientID = exp("spec.target.registration.oidc.clientId", r.OIDC.ClientID)
			o.Audience = exp("spec.target.registration.oidc.audience", r.OIDC.Audience)
			reg.OIDC = &o
		}
		s.Target.Registration = &reg
	}

	if len(h.Spec.Providers) > 0 {
		s.Providers = make([]Provider, len(h.Spec.Providers))
		for i, p := range h.Spec.Providers {
			np := p // copies Credentials (*SecretRef) through untouched
			base := fmt.Sprintf("spec.providers[%d]", i)
			np.Name = exp(base+".name", p.Name)
			np.Type = exp(base+".type", p.Type)
			np.Management = exp(base+".management", p.Management)
			if len(p.Config) > 0 {
				np.Config = make(map[string]string, len(p.Config))
				for k, v := range p.Config {
					np.Config[k] = exp(base+".config."+k, v)
				}
			}
			s.Providers[i] = np
		}
	}

	s.Inference.Route = exp("spec.inference.route", h.Spec.Inference.Route)
	s.Inference.Provider = exp("spec.inference.provider", h.Spec.Inference.Provider)
	s.Inference.Model = exp("spec.inference.model", h.Spec.Inference.Model)
	s.Inference.Timeout = exp("spec.inference.timeout", h.Spec.Inference.Timeout)
	// Validate the (now expanded) timeout once, here at resolve time, so the plan
	// diff and reconcile write can parse it without handling an error.
	if _, err := s.Inference.TimeoutSecs(); err != nil {
		errs = append(errs, fmt.Sprintf("spec.inference.timeout: %v", err))
	}
	if v := h.Spec.Inference.Verify; v != nil {
		b := *v // copy so the resolved struct never aliases the input's *bool
		s.Inference.Verify = &b
	}

	s.Sandbox.Image = exp("spec.sandbox.image", h.Spec.Sandbox.Image)
	if p := h.Spec.Sandbox.Policy; p != nil {
		np := *p // copy so the resolved struct never aliases the input's PolicyRef
		np.File = exp("spec.sandbox.policy.file", p.File)
		s.Sandbox.Policy = &np
	}
	if len(h.Spec.Sandbox.Providers) > 0 {
		s.Sandbox.Providers = make([]string, len(h.Spec.Sandbox.Providers))
		for i, p := range h.Spec.Sandbox.Providers {
			s.Sandbox.Providers[i] = exp(fmt.Sprintf("spec.sandbox.providers[%d]", i), p)
		}
	}
	if len(h.Spec.Sandbox.Env) > 0 {
		s.Sandbox.Env = make(map[string]string, len(h.Spec.Sandbox.Env))
		for k, v := range h.Spec.Sandbox.Env {
			s.Sandbox.Env[k] = exp("spec.sandbox.env."+k, v)
		}
	}

	s.Agent.Type = exp("spec.agent.type", h.Spec.Agent.Type)
	s.Agent.Model = exp("spec.agent.model", h.Spec.Agent.Model)
	if len(h.Spec.Agent.Args) > 0 {
		s.Agent.Args = make([]string, len(h.Spec.Agent.Args))
		for i, a := range h.Spec.Agent.Args {
			s.Agent.Args[i] = exp(fmt.Sprintf("spec.agent.args[%d]", i), a)
		}
	}

	s.Source.Repo = exp("spec.source.repo", h.Spec.Source.Repo)
	s.Source.Ref = exp("spec.source.ref", h.Spec.Source.Ref)
	s.Source.Destination = exp("spec.source.destination", h.Spec.Source.Destination)
	s.Source.Submodules = exp("spec.source.submodules", h.Spec.Source.Submodules)

	if len(h.Spec.Payloads) > 0 {
		s.Payloads = make([]Payload, len(h.Spec.Payloads))
		for i, p := range h.Spec.Payloads {
			base := fmt.Sprintf("spec.payloads[%d]", i)
			s.Payloads[i] = Payload{
				Source:      exp(base+".source", p.Source),
				Content:     exp(base+".content", p.Content),
				Destination: exp(base+".destination", p.Destination),
			}
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return &resolved, nil
}
