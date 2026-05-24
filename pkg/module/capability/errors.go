// SPDX-License-Identifier: Apache-2.0

package capability

import "errors"

// Scoping-violation sentinels. A capability backend returns one of
// these when a module's request falls outside its manifest-declared
// scope; the task-2 Invoker turns the returned error into an
// audited failure entry.
var (
	ErrPathDenied       = errors.New("capability: path outside allowed scope")
	ErrDomainDenied     = errors.New("capability: http domain not allowed")
	ErrCommandDenied    = errors.New("capability: command not allowed")
	ErrSecretPathDenied = errors.New("capability: secret path outside allowed scope")
	ErrSizeExceeded     = errors.New("capability: size limit exceeded")
	ErrRateLimited      = errors.New("capability: rate limit exceeded")
	ErrHostUnavailable  = errors.New("capability: host backend not wired")
)
