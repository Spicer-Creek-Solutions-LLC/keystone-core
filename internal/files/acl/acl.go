package acl

import (
	"context"
	"errors"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// ACL is the single seam the transport layer talks to. A Service
// constructed without an ACL behaves as if every request is
// allowed; production deployments wire a non-nil ACL at boot.
type ACL interface {
	// Authorize reports whether principal may perform op against
	// namespace. A nil return means allowed; non-nil means denied.
	// [ErrForbidden] (or an error wrapping it) is the canonical
	// denial; any other error is treated as a transport-layer
	// failure (logged, surfaced to caller as a 500-equivalent).
	Authorize(ctx context.Context, principal *auth.Principal, op files.FileOperation, namespace string) error
}

// ErrForbidden is the canonical sentinel returned from
// [ACL.Authorize] when a request is denied for authorization
// reasons (the principal lacks the required role, the namespace
// is closed, etc.). Use [errors.Is] to detect it.
var ErrForbidden = errors.New("acl: forbidden")
