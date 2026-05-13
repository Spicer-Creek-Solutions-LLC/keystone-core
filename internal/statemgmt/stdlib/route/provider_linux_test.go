//go:build linux

package route

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

const sampleRouteJSON = `[{"dst":"0.0.0.0/0","gateway":"192.168.1.1","dev":"eth0","metric":100}]`

func TestLinuxProvider_GetRoute_Present(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider(sampleRouteJSON, nil)
	r, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main", Metric: 100, HasMetric: true})
	if err != nil || r == nil {
		t.Fatalf("got %+v %v", r, err)
	}
	if r.Gateway != "192.168.1.1" || r.Interface != "eth0" || !r.HasMetric || r.Metric != 100 {
		t.Errorf("entry: %+v", r)
	}
	if (*calls)[0].bin != "ip" || strings.Join((*calls)[0].args, " ") != "-j route show table main exact 0.0.0.0/0 metric 100" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// without HasMetric
	p, calls = newRecordingProvider(sampleRouteJSON, nil)
	if _, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-j route show table main exact 0.0.0.0/0" {
		t.Errorf("no-metric args: %+v", (*calls)[0])
	}
}

func TestLinuxProvider_GetRoute_Absent(t *testing.T) {
	t.Parallel()
	// empty array → nil entry, no error
	p, _ := newRecordingProvider("[]", nil)
	r, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main"})
	if err != nil || r != nil {
		t.Errorf("absent: %+v %v", r, err)
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("ip route show: exit 2"))
	if _, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main"}); err == nil {
		t.Error("runner error should propagate")
	}
	// non-JSON → parse error
	p, _ = newRecordingProvider("not-json", nil)
	if _, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main"}); err == nil {
		t.Error("non-JSON → parse error")
	}
	// missing ip binary
	p = &linuxProvider{}
	if _, err := p.GetRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0"}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestNormaliseDst(t *testing.T) {
	t.Parallel()
	if normaliseDst("default") != "0.0.0.0/0" {
		t.Error("default → 0.0.0.0/0")
	}
	if normaliseDst("10.0.0.0/24") != "10.0.0.0/24" {
		t.Error("passthrough")
	}
}

func TestLinuxProvider_ReplaceRoute(t *testing.T) {
	t.Parallel()
	// default route via gateway
	p, calls := newRecordingProvider("", nil)
	err := p.ReplaceRoute(context.Background(), RouteSpec{
		Destination: "0.0.0.0/0", Gateway: "192.168.1.1", Table: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "route replace 0.0.0.0/0 via 192.168.1.1 table main" {
		t.Errorf("default-via: %+v", (*calls)[0])
	}
	// with interface + metric
	p, calls = newRecordingProvider("", nil)
	err = p.ReplaceRoute(context.Background(), RouteSpec{
		Destination: "10.0.0.0/24", Gateway: "192.168.1.1", Interface: "eth0",
		Metric: 100, HasMetric: true, Table: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "route replace 10.0.0.0/24 via 192.168.1.1 dev eth0 metric 100 table main" {
		t.Errorf("full: %+v", (*calls)[0])
	}
	// interface only (no gateway)
	p, calls = newRecordingProvider("", nil)
	err = p.ReplaceRoute(context.Background(), RouteSpec{
		Destination: "10.0.0.0/24", Interface: "eth0", Table: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "route replace 10.0.0.0/24 dev eth0 table main" {
		t.Errorf("dev-only: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("EEXIST"))
	if err := p.ReplaceRoute(context.Background(), RouteSpec{Destination: "0.0.0.0/0", Gateway: "192.168.1.1", Table: "main"}); err == nil {
		t.Error("runner error should propagate")
	}
	// missing ip
	p = &linuxProvider{}
	if err := p.ReplaceRoute(context.Background(), RouteSpec{Destination: "0.0.0.0/0", Gateway: "192.168.1.1"}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestLinuxProvider_DelRoute(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.DelRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "route del 0.0.0.0/0 table main" {
		t.Errorf("del: %+v", (*calls)[0])
	}
	// with metric
	p, calls = newRecordingProvider("", nil)
	if err := p.DelRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0", Table: "main", Metric: 100, HasMetric: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "route del 0.0.0.0/0 metric 100 table main" {
		t.Errorf("del with metric: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.DelRoute(context.Background(), RouteQuery{Destination: "0.0.0.0/0"}); !errors.Is(err, ErrNoIP) {
		t.Errorf("missing ip → %v", err)
	}
}

func TestRouteTable_DefaultFold(t *testing.T) {
	t.Parallel()
	if routeTable("") != "main" {
		t.Error("empty → main")
	}
	if routeTable("vpn") != "vpn" {
		t.Error("passthrough")
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
