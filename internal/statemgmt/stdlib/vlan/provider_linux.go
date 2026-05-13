//go:build linux

package vlan

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

type linkShow struct {
	Ifname   string `json:"ifname"`
	LinkInfo struct {
		InfoKind string `json:"info_kind"`
	} `json:"linkinfo"`
}

func (p *linuxProvider) GetLink(ctx context.Context, name string) (*LinkInfo, error) {
	bin, err := p.ip()
	if err != nil {
		return nil, err
	}
	out, runErr := p.run(ctx, bin, []string{"-d", "-j", "link", "show", "dev", name})
	if runErr != nil {
		if isNotFound(runErr) {
			return nil, nil
		}
		return nil, runErr
	}
	var arr []linkShow
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse `ip -d -j link show`: %w", err)
	}
	if len(arr) == 0 {
		return nil, nil
	}
	return &LinkInfo{Name: arr[0].Ifname, Kind: arr[0].LinkInfo.InfoKind}, nil
}

func isNotFound(err error) bool {
	s := err.Error()
	return strings.Contains(s, "does not exist") || strings.Contains(s, "Cannot find device")
}

func (p *linuxProvider) CreateVLAN(ctx context.Context, s VLANSpec) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	args := []string{"link", "add", "link", s.Parent, "name", s.Name, "type", "vlan", "id", strconv.Itoa(s.ID)}
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) DeleteLink(ctx context.Context, name string) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"link", "del", name})
	return err
}

func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed iproute2 flags + validated interface names + numeric VLAN ID from a validated declaration
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
