// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package agent

import (
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// applyUserCredential resolves username via os/user.Lookup and sets
// cmd.SysProcAttr.Credential so the spawned process runs as that
// user (Linux uid/gid).
//
// Requires the agent process to have CAP_SETUID — typically root,
// or a systemd unit with explicit AmbientCapabilities=. On v1.0
// Linux deployments the agent runs as root by default; non-root
// runs of Execute(req{User: ...}) will fail at fork-exec time with
// EPERM, surfaced via Result.Error.
func applyUserCredential(cmd *exec.Cmd, username string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("executor: user lookup %q: %w", username, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return fmt.Errorf("executor: parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return fmt.Errorf("executor: parse gid %q: %w", u.Gid, err)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}
	return nil
}
