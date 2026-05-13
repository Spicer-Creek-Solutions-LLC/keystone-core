//go:build linux

package vlan

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

func TestLinuxProvider_GetLink(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider(`[{"ifname":"eth0.10","linkinfo":{"info_kind":"vlan"}}]`, nil)
	link, err := p.GetLink(context.Background(), "eth0.10")
	if err != nil || link == nil || link.Kind != "vlan" {
		t.Fatalf("%+v %v", link, err)
	}
	if strings.Join((*calls)[0].args, " ") != "-d -j link show dev eth0.10" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// not found
	p, _ = newRecordingProvider("", errors.New(`Device "eth0.10" does not exist.`))
	if link, err := p.GetLink(context.Background(), "eth0.10"); err != nil || link != nil {
		t.Errorf("not-found: %+v %v", link, err)
	}
	// other error
	p, _ = newRecordingProvider("", errors.New("EPERM"))
	if _, err := p.GetLink(context.Background(), "eth0.10"); err == nil {
		t.Error("other error should propagate")
	}
	// empty array
	p, _ = newRecordingProvider("[]", nil)
	if link, _ := p.GetLink(context.Background(), "eth0.10"); link != nil {
		t.Error("empty array")
	}
	// non-JSON
	p, _ = newRecordingProvider("not-json", nil)
	if _, err := p.GetLink(context.Background(), "eth0.10"); err == nil {
		t.Error("non-JSON should error")
	}
	// missing ip
	p = &linuxProvider{}
	if _, err := p.GetLink(context.Background(), "eth0.10"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if !isNotFound(errors.New(`Device "x" does not exist.`)) {
		t.Error("does-not-exist")
	}
	if !isNotFound(errors.New("Cannot find device x")) {
		t.Error("cannot-find")
	}
	if isNotFound(errors.New("EPERM")) {
		t.Error("unrelated")
	}
}

func TestLinuxProvider_CreateVLAN(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	err := p.CreateVLAN(context.Background(), VLANSpec{Name: "eth0.10", Parent: "eth0", ID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link add link eth0 name eth0.10 type vlan id 10" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("EEXIST"))
	if err := p.CreateVLAN(context.Background(), VLANSpec{Name: "x", Parent: "eth0", ID: 1}); err == nil {
		t.Error("runner error should propagate")
	}
	// missing ip
	p = &linuxProvider{}
	if err := p.CreateVLAN(context.Background(), VLANSpec{Name: "x", Parent: "eth0", ID: 1}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_DeleteLink(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.DeleteLink(context.Background(), "eth0.10"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link del eth0.10" {
		t.Errorf("del: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.DeleteLink(context.Background(), "x"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("false")
	}
	if _, err := execRun(context.Background(), "/nonexistent/ip", nil); err == nil {
		t.Error("missing")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil || out != "ok" {
		t.Errorf("echo: %q %v", out, err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("nil")
	}
}
