//go:build linux

package bridge

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
	// bridge kind
	p, calls := newRecordingProvider(`[{"ifname":"br0","linkinfo":{"info_kind":"bridge"}}]`, nil)
	link, err := p.GetLink(context.Background(), "br0")
	if err != nil || link == nil || link.Kind != "bridge" {
		t.Fatalf("%+v %v", link, err)
	}
	if strings.Join((*calls)[0].args, " ") != "-d -j link show dev br0" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// not found
	p, _ = newRecordingProvider("", errors.New(`Device "br0" does not exist.`))
	if link, err := p.GetLink(context.Background(), "br0"); err != nil || link != nil {
		t.Errorf("not-found: %+v %v", link, err)
	}
	// other error
	p, _ = newRecordingProvider("", errors.New("EPERM"))
	if _, err := p.GetLink(context.Background(), "br0"); err == nil {
		t.Error("other error should propagate")
	}
	// empty array
	p, _ = newRecordingProvider("[]", nil)
	if link, err := p.GetLink(context.Background(), "br0"); err != nil || link != nil {
		t.Errorf("empty: %+v %v", link, err)
	}
	// non-JSON
	p, _ = newRecordingProvider("not-json", nil)
	if _, err := p.GetLink(context.Background(), "br0"); err == nil {
		t.Error("non-JSON should error")
	}
	// missing ip
	p = &linuxProvider{}
	if _, err := p.GetLink(context.Background(), "br0"); !errors.Is(err, ErrNoIP) {
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

func TestLinuxProvider_CreateBridge(t *testing.T) {
	t.Parallel()
	// without stp
	p, calls := newRecordingProvider("", nil)
	if err := p.CreateBridge(context.Background(), BridgeSpec{Name: "br0"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link add br0 type bridge" {
		t.Errorf("plain: %+v", (*calls)[0])
	}
	// with stp
	p, calls = newRecordingProvider("", nil)
	if err := p.CreateBridge(context.Background(), BridgeSpec{Name: "br0", STP: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link add br0 type bridge stp_state 1" {
		t.Errorf("stp: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("EEXIST"))
	if err := p.CreateBridge(context.Background(), BridgeSpec{Name: "br0"}); err == nil {
		t.Error("runner error should propagate")
	}
	// missing ip
	p = &linuxProvider{}
	if err := p.CreateBridge(context.Background(), BridgeSpec{Name: "br0"}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_DeleteLink(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.DeleteLink(context.Background(), "br0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link del br0" {
		t.Errorf("del: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.DeleteLink(context.Background(), "br0"); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_SetMaster(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.SetMaster(context.Background(), "eth0", "br0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "link set eth0 master br0" {
		t.Errorf("master: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.SetMaster(context.Background(), "eth0", "br0"); !errors.Is(err, ErrNoIP) {
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
