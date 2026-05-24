// SPDX-License-Identifier: Apache-2.0

//go:build windows

package agent

import (
	"errors"
	"os/exec"
)

// applyUserCredential is a stub on Windows — v1.0 ships Linux-only
// agents (PROJECT-DETAILS §4.6: Windows agent is post-v1.0). Returning an
// error when User != "" surfaces a clear "not supported" message
// rather than a silent no-op.
func applyUserCredential(_ *exec.Cmd, _ string) error {
	return errors.New("executor: user switching not supported on Windows in v1.0 (see PROJECT-DETAILS §4.6)")
}
