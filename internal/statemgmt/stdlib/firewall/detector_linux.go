//go:build linux

package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func defaultDetector() BackendDetector {
	d := &linuxDetector{run: execRun}
	d.firewallCmdBin, _ = exec.LookPath("firewall-cmd")
	d.iptablesBin, _ = exec.LookPath("iptables")
	d.nftBin, _ = exec.LookPath("nft")
	return d
}

type linuxDetector struct {
	firewallCmdBin string // resolved firewall-cmd path ("" if absent)
	iptablesBin    string
	nftBin         string
	run            commandRunner
}

// Detect picks the active firewall backend in this order:
//
//  1. `firewall-cmd` exists AND `firewall-cmd --state` exits 0 →
//     firewalld is running and managing the system.
//  2. `iptables` on PATH → iptables (covers iptables-legacy *and*
//     iptables-nft; pure-nft setups should pin `backend: nftables`
//     explicitly).
//  3. `nft` on PATH → nftables.
//  4. else → ErrNoFirewall.
func (d *linuxDetector) Detect(ctx context.Context) (string, error) {
	if d.firewallCmdBin != "" {
		if _, err := d.run(ctx, d.firewallCmdBin, []string{"--state"}); err == nil {
			return BackendFirewalld, nil
		}
	}
	if d.iptablesBin != "" {
		return BackendIptables, nil
	}
	if d.nftBin != "" {
		return BackendNftables, nil
	}
	return "", ErrNoFirewall
}

// execRun is the production commandRunner. The detector only ever
// runs `firewall-cmd --state`, so the only thing we care about is
// exit status — but we still capture combined output for symmetry
// with the rest of the stdlib firewall providers.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed firewall-cmd flags
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
