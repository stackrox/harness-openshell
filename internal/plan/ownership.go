package plan

import "github.com/stackrox/harness-openshell/internal/openshell"

// Ownership labels mark a provider as reconcile-managed by this harness. They are
// the single vocabulary for "the harness owns this provider" — the diff rule
// (ProviderAction) reads them to decide adoption, and reconcile stamps them on
// the providers it updates. Kept here as the one owner so the plan and the write
// path can never disagree on what "owned" means.
const (
	// OwnerLabelKey is the label key stamped on harness-managed providers.
	OwnerLabelKey = "harness.openshell.dev/managed-by"
	// OwnerLabelValue is the value OwnerLabelKey must carry to count as owned.
	OwnerLabelValue = "harness"
)

// IsOwned reports whether the provider carries this harness's ownership label.
// It checks both key and value: a foreign managed-by value (another controller)
// is deliberately not ours, so reconcile will not silently take it over.
func IsOwned(p openshell.Provider) bool {
	return p.Labels[OwnerLabelKey] == OwnerLabelValue
}
