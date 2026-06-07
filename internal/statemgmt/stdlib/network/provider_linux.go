// SPDX-License-Identifier: Apache-2.0

//go:build linux

package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func defaultProvider() Provider {
	p := &linuxProvider{run: execRun}
	p.ipBin, _ = exec.LookPath("ip")
	return p
}

type linuxProvider struct {
	ipBin string
	run   commandRunner
}

func (p *linuxProvider) ip() (string, error) {
	if p.ipBin == "" {
		return "", ErrNoIP
	}
	return p.ipBin, nil
}

// addrShowOutput is the relevant subset of `ip -j addr show dev <name>`
// output. The tool returns an array of objects (we ask for one dev,
// so length is 0 or 1).
type addrShowOutput struct {
	Ifname   string   `json:"ifname"`
	Flags    []string `json:"flags"`
	MTU      int      `json:"mtu"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

func (p *linuxProvider) GetInterface(ctx context.Context, name string) (*InterfaceState, error) {
	bin, err := p.ip()
	if err != nil {
		return nil, err
	}
	out, runErr := p.run(ctx, bin, []string{"-j", "addr", "show", "dev", name})
	if runErr != nil {
		if isNotFound(runErr) {
			return nil, fmt.Errorf("%w: %s", ErrInterfaceNotFound, name)
		}
		return nil, runErr
	}
	var arr []addrShowOutput
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse `ip -j addr show dev %s`: %w", name, err)
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrInterfaceNotFound, name)
	}
	entry := arr[0]
	state := &InterfaceState{
		Name: entry.Ifname,
		Up:   hasFlag(entry.Flags, "UP"),
		MTU:  entry.MTU,
	}
	for _, ai := range entry.AddrInfo {
		if ai.Local == "" || ai.PrefixLen == 0 {
			// PrefixLen 0 is /0 which is technically valid but iproute
			// omits it for normal addresses; skip empties defensively.
			if ai.Local == "" {
				continue
			}
		}
		cidr := ai.Local + "/" + strconv.Itoa(ai.PrefixLen)
		canon, err := canonicalCIDR(cidr)
		if err != nil {
			// Skip unparseable entries rather than failing the whole
			// check; the operator will see the addresses we did parse.
			continue
		}
		state.Addresses = append(state.Addresses, canon)
	}
	return state, nil
}

// isNotFound reports whether an `ip` invocation failed because the
// requested device doesn't exist. `ip` prints "Device "X" does not
// exist." to stderr and exits non-zero; execRun's error string
// carries the combined output.
func isNotFound(err error) bool {
	s := err.Error()
	return strings.Contains(s, "does not exist") || strings.Contains(s, "Cannot find device")
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if f == want {
			return true
		}
	}
	return false
}

func (p *linuxProvider) AddAddress(ctx context.Context, name, cidr string) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"addr", "add", cidr, "dev", name})
	return err
}

func (p *linuxProvider) DelAddress(ctx context.Context, name, cidr string) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"addr", "del", cidr, "dev", name})
	return err
}

func (p *linuxProvider) SetMTU(ctx context.Context, name string, mtu int) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"link", "set", "dev", name, "mtu", strconv.Itoa(mtu)})
	return err
}

func (p *linuxProvider) SetLinkUp(ctx context.Context, name string, up bool) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	action := "up"
	if !up {
		action = "down"
	}
	_, err = p.run(ctx, bin, []string{"link", "set", "dev", name, action})
	return err
}

// execRun is the production commandRunner. Captures combined output
// so the kernel's complaint (e.g. "Cannot find device") reaches the
// caller in err.Error().
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed iproute2 flags + a validated interface name + a canonicalised CIDR or numeric MTU from a validated declaration
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
