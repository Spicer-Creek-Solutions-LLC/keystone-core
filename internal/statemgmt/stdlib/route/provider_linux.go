// SPDX-License-Identifier: Apache-2.0

//go:build linux

package route

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

// routeShowEntry is the relevant subset of `ip -j route show …`
// output. Fields not present in the JSON for a given route default
// to the zero value, which we filter / fold later.
type routeShowEntry struct {
	Dst     string `json:"dst"`
	Gateway string `json:"gateway"`
	Dev     string `json:"dev"`
	Metric  int    `json:"metric"`
}

func (p *linuxProvider) GetRoute(ctx context.Context, q RouteQuery) (*RouteEntry, error) {
	bin, err := p.ip()
	if err != nil {
		return nil, err
	}
	args := []string{"-j", "route", "show"}
	args = append(args, "table", routeTable(q.Table))
	args = append(args, "exact", q.Destination)
	if q.HasMetric {
		args = append(args, "metric", strconv.Itoa(q.Metric))
	}
	out, runErr := p.run(ctx, bin, args)
	if runErr != nil {
		return nil, runErr
	}
	var arr []routeShowEntry
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse `ip -j route show`: %w", err)
	}
	if len(arr) == 0 {
		return nil, nil
	}
	// `ip route show exact <dest>` may still return multiple entries
	// (different metrics, different protocols). When the operator
	// declared a metric, we filtered server-side via `metric N`; pick
	// the first remaining. When no metric was declared, we still pick
	// the first — operators who care about specific routes should
	// declare a metric.
	entry := arr[0]
	r := &RouteEntry{
		Destination: normaliseDst(entry.Dst),
		Gateway:     entry.Gateway,
		Interface:   entry.Dev,
		Table:       routeTable(q.Table),
	}
	if entry.Metric != 0 {
		r.Metric = entry.Metric
		r.HasMetric = true
	}
	return r, nil
}

// normaliseDst translates the kernel's stringified destination back
// to a canonical CIDR — `ip route show` prints "default" for
// 0.0.0.0/0, "0.0.0.0/0" otherwise.
func normaliseDst(s string) string {
	if s == "default" {
		// Caller already knows whether they're querying v4 or v6
		// by what they declared; we just return the declared CIDR
		// upstream. Returning "default" here is harmless because
		// the route entry's destination is informational — it isn't
		// used for equality (the lookup key was).
		return "0.0.0.0/0"
	}
	return s
}

func (p *linuxProvider) ReplaceRoute(ctx context.Context, s RouteSpec) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	args := []string{"route", "replace", s.Destination}
	if s.Gateway != "" {
		args = append(args, "via", s.Gateway)
	}
	if s.Interface != "" {
		args = append(args, "dev", s.Interface)
	}
	if s.HasMetric {
		args = append(args, "metric", strconv.Itoa(s.Metric))
	}
	args = append(args, "table", routeTable(s.Table))
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) DelRoute(ctx context.Context, q RouteQuery) error {
	bin, err := p.ip()
	if err != nil {
		return err
	}
	args := []string{"route", "del", q.Destination}
	if q.HasMetric {
		args = append(args, "metric", strconv.Itoa(q.Metric))
	}
	args = append(args, "table", routeTable(q.Table))
	_, err = p.run(ctx, bin, args)
	return err
}

func routeTable(t string) string {
	if t == "" {
		return "main"
	}
	return t
}

// execRun is the production commandRunner.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed iproute2 flags + canonicalised CIDR/IP and validated interface/table names from a validated declaration
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
