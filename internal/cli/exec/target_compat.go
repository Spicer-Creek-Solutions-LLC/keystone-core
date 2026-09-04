// SPDX-License-Identifier: Apache-2.0

package exec

import (
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"

	"go.keystone-core.io/keystone-core/internal/cli/target"
)

// ErrTargetUnsupported is re-exported so callers and tests keep the
// symbol they had when the parser lived in this package.
var ErrTargetUnsupported = target.ErrTargetUnsupported

// ParseTarget delegates to internal/cli/target.
//
// The parser moved out when `state` needed the same targeting dialect:
// exec and state must accept identical target expressions, and the way
// to guarantee that is one parser, not two that agree today.
func ParseTarget(raw string) (*v1.Target, error) { return target.ParseTarget(raw) }
