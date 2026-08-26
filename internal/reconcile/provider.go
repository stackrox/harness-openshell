package reconcile

import (
	"context"
	"errors"
	"fmt"

	"github.com/stackrox/harness-openshell/internal/config"
	"github.com/stackrox/harness-openshell/internal/openshell"
	"github.com/stackrox/harness-openshell/internal/plan"
)

// ProviderResult reports what ReconcileProviders decided for one provider and the
// resulting (or current) firewall view of it.
//
// Provider holds: the gateway's response on Update; the current provider on Noop
// and on AdoptionRequired (read at diff time); and a bare {Name, Type} echo on
// Create, which reconcile deliberately does NOT write (invariant 26 — credentialed
// creation is the CLI bridge's job, done upstream by providerCreatePlan).
type ProviderResult struct {
	Name     string
	Action   plan.Action
	Provider openshell.Provider
}

// ReconcileProviders drives each desired provider toward the gateway state,
// routing every decision through the shared plan.ProviderAction rule so the
// read-only plan and this write path can never disagree (invariant 22). Like
// ReconcileInference it does not degrade: any non-NotFound read error, or any
// write error, is returned so the caller learns the reconcile did not complete.
//
// It never creates a credentialed provider and never deletes (invariant 26):
//   - Create (managed absent) is reported without writing; providerCreatePlan
//     (the CLI bridge, S6) does the credentialed create, after which a re-run
//     sees the provider present.
//   - AdoptionRequired for an existing-but-unowned provider is reported without
//     writing (drift the operator must resolve with `adopt: true`).
//   - AdoptionRequired for an ABSENT referenced provider is a hard error: a
//     referenced provider that does not exist is unusable.
//
// On Update the write is credential-preserving by construction (the firewall
// Provider has no credentials field) and is reached only on a real non-secret
// delta, so the empty-credential copy-through is never sent spuriously.
func ReconcileProviders(ctx context.Context, c openshell.Client, desired []config.Provider) ([]ProviderResult, error) {
	results := make([]ProviderResult, 0, len(desired))

	for _, d := range desired {
		cur, err := c.GetProvider(ctx, d.Name)
		var curPtr *openshell.Provider
		switch {
		case err == nil:
			curPtr = &cur
		case errors.Is(err, openshell.ErrNotFound):
			curPtr = nil
		default:
			return nil, fmt.Errorf("reading provider %q: %w", d.Name, err)
		}

		action := plan.ProviderAction(d, curPtr)
		switch action {
		case plan.ActionNoop:
			// cur is guaranteed present here (Noop implies an existing provider); a
			// successful Get is itself the referenced-present verification.
			results = append(results, ProviderResult{Name: d.Name, Action: action, Provider: cur})

		case plan.ActionUpdate:
			updated, err := c.UpdateProvider(ctx, managedProvider(d, curPtr))
			if err != nil {
				return nil, fmt.Errorf("updating provider %q: %w", d.Name, err)
			}
			results = append(results, ProviderResult{Name: d.Name, Action: action, Provider: updated})

		case plan.ActionCreate:
			// Managed absent: report the intended create; do NOT SDK-create.
			results = append(results, ProviderResult{
				Name: d.Name, Action: action,
				Provider: openshell.Provider{Name: d.Name, Type: d.Type},
			})

		case plan.ActionAdoptionRequired:
			if curPtr == nil {
				// Referenced (or unknown-management) but absent: unusable.
				return nil, fmt.Errorf("referenced provider %q does not exist: %w", d.Name, openshell.ErrNotFound)
			}
			// Exists but unowned and not adopted: report drift, write nothing.
			results = append(results, ProviderResult{Name: d.Name, Action: action, Provider: cur})

		default:
			return nil, fmt.Errorf("unexpected provider action %q for %q", action, d.Name)
		}
	}

	return results, nil
}

// managedProvider builds the openshell.Provider written on an Update. It merges
// the desired Config and the owner label ONTO the current provider's fields
// rather than replacing them, so an update triggered by one managed key never
// wipes config keys or labels the harness does not manage. This keeps the write
// consistent with the diff rule's subset semantics (plan.configDrifts): the
// harness owns only the keys it declares. The owner label is always stamped —
// on first adoption that stamp is itself the Label delta that made this an
// Update. cur is non-nil here (Update implies an existing provider).
func managedProvider(d config.Provider, cur *openshell.Provider) openshell.Provider {
	cfg := map[string]string{}
	for k, v := range cur.Config {
		cfg[k] = v
	}
	for k, v := range d.Config {
		cfg[k] = v
	}

	labels := map[string]string{}
	for k, v := range cur.Labels {
		labels[k] = v
	}
	labels[plan.OwnerLabelKey] = plan.OwnerLabelValue

	return openshell.Provider{Name: d.Name, Type: d.Type, Config: cfg, Labels: labels}
}
