package openshell

import "errors"

// Sentinel errors are the harness-owned meanings of gateway failures. Callers
// branch on these via errors.Is; they never inspect SDK error types. The single
// owner of the SDK-error → sentinel mapping is sdkclient.translate.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("openshell: not found")
	// ErrUnavailable is returned when the gateway cannot be reached.
	ErrUnavailable = errors.New("openshell: gateway unavailable")
	// ErrUnauthenticated is returned when the caller's credentials are missing
	// or rejected.
	ErrUnauthenticated = errors.New("openshell: unauthenticated")
	// ErrPermission is returned when the caller is authenticated but not
	// authorized.
	ErrPermission = errors.New("openshell: permission denied")
	// ErrUnsupported is returned when the gateway does not implement the
	// requested RPC (gRPC Unimplemented).
	ErrUnsupported = errors.New("openshell: not supported by gateway")
	// ErrInvalidArgument is returned when the caller supplied invalid input
	// (e.g. a required field left empty), rejected by the gateway before any
	// state change (gRPC InvalidArgument).
	ErrInvalidArgument = errors.New("openshell: invalid argument")
	// ErrConfig is returned when the gateway config cannot be loaded, parsed, or
	// its auth mode cannot be satisfied.
	ErrConfig = errors.New("openshell: gateway config error")
)
