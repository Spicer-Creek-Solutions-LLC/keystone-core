//go:build linux

package firewall

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

func newRecordingDetector(fwOK bool) (*linuxDetector, *[]capture) {
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		if fwOK {
			return "running\n", nil
		}
		return "", errors.New("firewall-cmd --state: exit 252: not running")
	}
	return &linuxDetector{run: run}, &calls
}

func TestLinuxDetector_FirewalldActive(t *testing.T) {
	t.Parallel()
	d, calls := newRecordingDetector(true)
	d.firewallCmdBin = "firewall-cmd"
	d.iptablesBin = "iptables" // both present — firewalld wins when active
	got, err := d.Detect(context.Background())
	if err != nil || got != BackendFirewalld {
		t.Fatalf("Detect = %q,%v", got, err)
	}
	if len(*calls) != 1 || (*calls)[0].bin != "firewall-cmd" || strings.Join((*calls)[0].args, " ") != "--state" {
		t.Errorf("expected firewall-cmd --state call, got %+v", *calls)
	}
}

func TestLinuxDetector_FirewalldInstalledButStopped_FallsToIptables(t *testing.T) {
	t.Parallel()
	d, _ := newRecordingDetector(false)
	d.firewallCmdBin = "firewall-cmd"
	d.iptablesBin = "iptables"
	got, err := d.Detect(context.Background())
	if err != nil || got != BackendIptables {
		t.Errorf("stopped firewalld + iptables → %q,%v", got, err)
	}
}

func TestLinuxDetector_NoFirewalld_HasIptables(t *testing.T) {
	t.Parallel()
	d, _ := newRecordingDetector(false)
	d.iptablesBin = "iptables"
	got, err := d.Detect(context.Background())
	if err != nil || got != BackendIptables {
		t.Errorf("only iptables → %q,%v", got, err)
	}
}

func TestLinuxDetector_OnlyNft(t *testing.T) {
	t.Parallel()
	d, _ := newRecordingDetector(false)
	d.nftBin = "nft"
	got, err := d.Detect(context.Background())
	if err != nil || got != BackendNftables {
		t.Errorf("only nft → %q,%v", got, err)
	}
}

func TestLinuxDetector_Nothing(t *testing.T) {
	t.Parallel()
	d, _ := newRecordingDetector(false)
	if _, err := d.Detect(context.Background()); !errors.Is(err, ErrNoFirewall) {
		t.Errorf("no backend → %v", err)
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/firewall-cmd", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("echo = %q", out)
	}
}

func TestDefaultDetector_NonNil(t *testing.T) {
	t.Parallel()
	if defaultDetector() == nil {
		t.Fatal("defaultDetector returned nil")
	}
}
