package sdkclient

import (
	"fmt"

	v1 "github.com/NVIDIA/OpenShell/sdk/go/openshell/v1"

	"github.com/stackrox/harness-openshell/internal/openshell"
)

// translate is the single owner of SDK-error meaning: it maps SDK typed errors
// to the harness sentinels. Every other package matches meaning with errors.Is
// on those sentinels and never inspects SDK error codes directly.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case v1.IsNotFound(err):
		return fmt.Errorf("%w: %v", openshell.ErrNotFound, err)
	case v1.IsUnavailable(err):
		return fmt.Errorf("%w: %v", openshell.ErrUnavailable, err)
	case v1.IsUnimplemented(err):
		return fmt.Errorf("%w: %v", openshell.ErrUnsupported, err)
	case v1.IsUnauthenticated(err):
		return fmt.Errorf("%w: %v", openshell.ErrUnauthenticated, err)
	case v1.IsPermissionDenied(err):
		return fmt.Errorf("%w: %v", openshell.ErrPermission, err)
	default:
		return err
	}
}
