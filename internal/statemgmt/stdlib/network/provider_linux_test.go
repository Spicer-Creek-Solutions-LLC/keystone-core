// SPDX-License-Identifier: Apache-2.0

//go:build linux

package network

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type capture struct {
	bin  string
	args []string
}

func newRecordingProvider(out string, runErr error) (*linuxProvider, *[]capture) {
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		return out, runErr
	}
	return &linuxProvider{ipBin: "ip", run: run}, &calls
}

const sampleAddrJSON = `[
  {
    "ifindex": 2,
    "ifname": "eth0",
    "flags": ["BROADCAST","MULTICAST","UP","LOWER_UP"],
    "mtu": 1500,
    "operstate": "UP",
    "link_type": "ether",
    "address": "aa:bb:cc:dd:ee:ff",
    "addr_info": [
      {"family":"inet","local":"192.168.1.10","prefixlen":24},
      {"family":"inet6","local":"fe80::1","prefixlen":64}
    ]
  }
]`

func TestLinuxProvider_GetInterface(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider(sampleAddrJSON, nil)
	state, err := p.GetInterface(context.Background(), "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != "eth0" || !state.Up || state.MTU != 1500 {
		t.Errorf("state: %+v", state)
	}
	sort.Strings(state.Addresses)
	want := []string{"192.168.1.10/24", "fe80::1/64"}
	sort.Strings(want)
	if !reflect.DeepEqual(state.Addresses, want) {
		t.Errorf("addresses: %+v want %+v", state.Addresses, want)
	}
	if strings.Join((*calls)[0].args, " ") != "-j addr show dev eth0" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// not found
	p, _ = newRecordingProvider("", errors.New(`exit 1: Device "missing" does not exist.`))
	if _, err := p.GetInterface(context.Background(), "missing"); !errors.Is(err, ErrInterfaceNotFound) {
		t.Errorf("not-found: %v", err)
	}
	// other error propagates
	p, _ = newRecordingProvider("", errors.New("exit 1: some other issue"))
	if _, err := p.GetInterface(context.Background(), "eth0"); err == nil || errors.Is(err, ErrInterfaceNotFound) {
		t.Errorf("other error: %v", err)
	}
	// empty array → not found
	p, _ = newRecordingProvider("[]", nil)
	if _, err := p.GetInterface(context.Background(), "eth0"); !errors.Is(err, ErrInterfaceNotFound) {
		t.Errorf("empty array → not-found: %v", err)
	}
	// non-JSON output
	p, _ = newRecordingProvider("not-json", nil)
	if _, err := p.GetInterface(context.Background(), "eth0"); err == nil {
		t.Error("non-JSON should error")
	}
	// down interface (no UP flag)
	p, _ = newRecordingProvider(`[{"ifname":"eth0","flags":["BROADCAST"],"mtu":1500,"addr_info":[]}]`, nil)
	s, err := p.GetInterface(context.Background(), "eth0")
	if err != nil || s.Up {
		t.Errorf("down: %+v %v", s, err)
	}
	// missing ip binary
	p = &linuxProvider{run: nil}
	if _, err := p.GetInterface(context.Background(), "eth0"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestHasFlag(t *testing.T) {
	t.Parallel()
	if !hasFlag([]string{"BROADCAST", "UP", "LOWER_UP"}, "UP") {
		t.Error("UP present")
	}
	if hasFlag([]string{"BROADCAST"}, "UP") {
		t.Error("UP not present")
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if !isNotFound(errors.New(`Device "eth0" does not exist.`)) {
		t.Error("does-not-exist")
	}
	if !isNotFound(errors.New("Cannot find device eth0")) {
		t.Error("cannot-find-device")
	}
	if isNotFound(errors.New("permission denied")) {
		t.Error("unrelated")
	}
}

func TestLinuxProvider_AddDelAddress(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.AddAddress(context.Background(), "eth0", "192.168.1.10/24"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "addr add 192.168.1.10/24 dev eth0" {
		t.Errorf("add: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.DelAddress(context.Background(), "eth0", "10.0.0.1/24"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "addr del 10.0.0.1/24 dev eth0" {
		t.Errorf("del: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("EEXIST"))
	if err := p.AddAddress(context.Background(), "eth0", "10.0.0.1/24"); err == nil {
		t.Error("add runner error should propagate")
	}
	// missing binary
	p = &linuxProvider{}
	if err := p.AddAddress(context.Background(), "eth0", "10.0.0.1/24"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip add → %v", err)
	}
	if err := p.DelAddress(context.Background(), "eth0", "10.0.0.1/24"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip del → %v", err)
	}
}

func TestLinuxProvider_SetMTU(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.SetMTU(context.Background(), "eth0", 9000); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link set dev eth0 mtu 9000" {
		t.Errorf("mtu: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.SetMTU(context.Background(), "eth0", 1500); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_SetLinkUp(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.SetLinkUp(context.Background(), "eth0", true); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link set dev eth0 up" {
		t.Errorf("up: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.SetLinkUp(context.Background(), "eth0", false); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link set dev eth0 down" {
		t.Errorf("down: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.SetLinkUp(context.Background(), "eth0", true); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/ip", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil || out != "ok" {
		t.Errorf("echo: %q %v", out, err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
