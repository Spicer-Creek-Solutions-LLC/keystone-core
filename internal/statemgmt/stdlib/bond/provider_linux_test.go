// SPDX-License-Identifier: Apache-2.0

//go:build linux

package bond

import (
	"context"
	"errors"
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

const sampleBondLinkJSON = `[{"ifname":"bond0","linkinfo":{"info_kind":"bond"}}]`
const samplePlainLinkJSON = `[{"ifname":"eth0","linkinfo":{}}]`

func TestLinuxProvider_GetLink(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider(sampleBondLinkJSON, nil)
	link, err := p.GetLink(context.Background(), "bond0")
	if err != nil || link == nil || link.Name != "bond0" || link.Kind != "bond" {
		t.Fatalf("bond: %+v %v", link, err)
	}
	if strings.Join((*calls)[0].args, " ") != "-d -j link show dev bond0" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// plain interface has no linkinfo.info_kind
	p, _ = newRecordingProvider(samplePlainLinkJSON, nil)
	link, _ = p.GetLink(context.Background(), "eth0")
	if link.Kind != "" {
		t.Errorf("plain kind: %q", link.Kind)
	}
	// not found
	p, _ = newRecordingProvider("", errors.New(`Device "bond0" does not exist.`))
	link, err = p.GetLink(context.Background(), "bond0")
	if err != nil || link != nil {
		t.Errorf("not-found: %+v %v", link, err)
	}
	// other error propagates
	p, _ = newRecordingProvider("", errors.New("exit 1: permission denied"))
	if _, err := p.GetLink(context.Background(), "bond0"); err == nil {
		t.Error("other error should propagate")
	}
	// empty array
	p, _ = newRecordingProvider("[]", nil)
	if link, err := p.GetLink(context.Background(), "bond0"); err != nil || link != nil {
		t.Errorf("empty array: %+v %v", link, err)
	}
	// non-JSON
	p, _ = newRecordingProvider("not-json", nil)
	if _, err := p.GetLink(context.Background(), "bond0"); err == nil {
		t.Error("non-JSON should error")
	}
	// missing ip
	p = &linuxProvider{}
	if _, err := p.GetLink(context.Background(), "bond0"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if !isNotFound(errors.New(`Device "x" does not exist.`)) {
		t.Error("does-not-exist")
	}
	if !isNotFound(errors.New("Cannot find device x")) {
		t.Error("cannot-find-device")
	}
	if isNotFound(errors.New("EPERM")) {
		t.Error("unrelated")
	}
}

func TestLinuxProvider_CreateBond(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	err := p.CreateBond(context.Background(), BondSpec{Name: "bond0", Mode: "active-backup"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link add bond0 type bond mode active-backup" {
		t.Errorf("create: %+v", (*calls)[0])
	}
	// with miimon
	p, calls = newRecordingProvider("", nil)
	err = p.CreateBond(context.Background(), BondSpec{Name: "bond0", Mode: "802.3ad", Miimon: 100, HasMiimon: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link add bond0 type bond mode 802.3ad miimon 100" {
		t.Errorf("create+miimon: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("EEXIST"))
	if err := p.CreateBond(context.Background(), BondSpec{Name: "bond0", Mode: "balance-rr"}); err == nil {
		t.Error("runner error should propagate")
	}
	// missing ip
	p = &linuxProvider{}
	if err := p.CreateBond(context.Background(), BondSpec{Name: "bond0", Mode: "balance-rr"}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_DeleteLink(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.DeleteLink(context.Background(), "bond0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link del bond0" {
		t.Errorf("del: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.DeleteLink(context.Background(), "bond0"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_SetMaster(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.SetMaster(context.Background(), "eth0", "bond0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link set eth0 master bond0" {
		t.Errorf("master: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.SetMaster(context.Background(), "eth0", "bond0"); !errors.Is(err, ErrNoIP) {
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
