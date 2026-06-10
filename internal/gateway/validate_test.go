package gateway

import (
	"fmt"
	"testing"
)

type stubProviderChecker struct {
	providers map[string]bool
}

func (s *stubProviderChecker) ProviderGet(name string) error {
	if s.providers[name] {
		return nil
	}
	return fmt.Errorf("not found")
}

func TestValidateProviders(t *testing.T) {
	gw := &stubProviderChecker{providers: map[string]bool{
		"github":       true,
		"vertex-local": true,
	}}

	registered, missing := ValidateProviders([]string{"github", "vertex-local", "atlassian"}, gw)
	if len(registered) != 2 || registered[0] != "github" || registered[1] != "vertex-local" {
		t.Errorf("registered = %v, want [github vertex-local]", registered)
	}
	if len(missing) != 1 || missing[0] != "atlassian" {
		t.Errorf("missing = %v, want [atlassian]", missing)
	}
}

func TestValidateProviders_Empty(t *testing.T) {
	gw := &stubProviderChecker{}
	registered, missing := ValidateProviders(nil, gw)
	if registered != nil || missing != nil {
		t.Errorf("registered = %v, missing = %v, want nil/nil", registered, missing)
	}
}
