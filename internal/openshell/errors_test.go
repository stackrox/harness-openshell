package openshell

import (
	"errors"
	"fmt"
	"testing"
)

// The sentinels must be distinct from one another and usable with errors.Is
// after wrapping, since sdkclient.translate wraps them with %w.
func TestSentinelsDistinctAndWrappable(t *testing.T) {
	sentinels := map[string]error{
		"ErrNotFound":        ErrNotFound,
		"ErrUnavailable":     ErrUnavailable,
		"ErrUnauthenticated": ErrUnauthenticated,
		"ErrPermission":      ErrPermission,
		"ErrUnsupported":     ErrUnsupported,
		"ErrConfig":          ErrConfig,
	}

	// Each sentinel is matched only by itself.
	for nameA, a := range sentinels {
		for nameB, b := range sentinels {
			match := errors.Is(a, b)
			want := nameA == nameB
			if match != want {
				t.Errorf("errors.Is(%s, %s) = %v, want %v", nameA, nameB, match, want)
			}
		}
	}

	// Wrapping preserves identity through errors.Is (the translate contract).
	for name, s := range sentinels {
		wrapped := fmt.Errorf("%w: underlying detail", s)
		if !errors.Is(wrapped, s) {
			t.Errorf("errors.Is(wrapped, %s) = false, want true", name)
		}
	}
}
