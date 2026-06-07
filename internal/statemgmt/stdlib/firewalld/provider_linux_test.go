// SPDX-License-Identifier: Apache-2.0

//go:build linux

package firewalld

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

func newRecordingProvider(out string, err error) (*linuxProvider, *[]capture) {
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		return out, err
	}
	return &linuxProvider{bin: "firewall-cmd", run: run}, &calls
}

func TestLinuxProvider_Has(t *testing.T) {
	t.Parallel()
	// --query exits 0 → present
	p, calls := newRecordingProvider("yes\n", nil)
	has, err := p.Has(context.Background(), "public", Item{Kind: KindService, Value: "ssh"})
	if err != nil || !has {
		t.Fatalf("Has(present) = %v,%v", has, err)
	}
	if strings.Join((*calls)[0].args, " ") != "--permanent --zone=public --query-service=ssh" {
		t.Errorf("query args: %v", (*calls)[0].args)
	}
	// non-zero exit → absent
	p, _ = newRecordingProvider("no\n", errors.New("exit 1: no"))
	has, err = p.Has(context.Background(), "public", Item{Kind: KindPort, Value: "8080/tcp"})
	if err != nil || has {
		t.Errorf("Has(absent) = %v,%v", has, err)
	}
	// missing firewall-cmd binary
	if _, err := (&linuxProvider{bin: "", run: nil}).Has(context.Background(), "public", Item{Kind: KindService, Value: "ssh"}); !errors.Is(err, ErrNoFirewallCmd) {
		t.Errorf("missing firewall-cmd → %v", err)
	}
}

func TestLinuxProvider_AddRemoveArgs(t *testing.T) {
	t.Parallel()
	// service add
	p, calls := newRecordingProvider("success\n", nil)
	if err := p.Add(context.Background(), "public", Item{Kind: KindService, Value: "ssh"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "--permanent --zone=public --add-service=ssh" {
		t.Errorf("add-service args: %v", (*calls)[0].args)
	}
	// port add on a custom zone
	p, calls = newRecordingProvider("success\n", nil)
	if err := p.Add(context.Background(), "dmz", Item{Kind: KindPort, Value: "8080/tcp"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "--permanent --zone=dmz --add-port=8080/tcp" {
		t.Errorf("add-port args: %v", (*calls)[0].args)
	}
	// rich-rule add — the value carries spaces and quotes
	p, calls = newRecordingProvider("success\n", nil)
	rule := `rule family="ipv4" source address="10.0.0.0/8" drop`
	if err := p.Add(context.Background(), "public", Item{Kind: KindRichRule, Value: rule}); err != nil {
		t.Fatal(err)
	}
	// firewall-cmd takes the rich rule as a single --add-rich-rule=… arg
	if len((*calls)[0].args) != 3 || (*calls)[0].args[2] != "--add-rich-rule="+rule {
		t.Errorf("add-rich-rule args: %v", (*calls)[0].args)
	}
	// remove
	p, calls = newRecordingProvider("success\n", nil)
	if err := p.Remove(context.Background(), "public", Item{Kind: KindService, Value: "ssh"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "--permanent --zone=public --remove-service=ssh" {
		t.Errorf("remove-service args: %v", (*calls)[0].args)
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("firewall-cmd: Error: INVALID_SERVICE"))
	if err := p.Add(context.Background(), "public", Item{Kind: KindService, Value: "nope"}); err == nil {
		t.Error("Add should propagate a runner error")
	}
	// missing firewall-cmd binary
	noFW := &linuxProvider{bin: "", run: nil}
	if err := noFW.Add(context.Background(), "public", Item{Kind: KindService, Value: "ssh"}); !errors.Is(err, ErrNoFirewallCmd) {
		t.Errorf("Add without firewall-cmd → %v", err)
	}
	if err := noFW.Remove(context.Background(), "public", Item{Kind: KindService, Value: "ssh"}); !errors.Is(err, ErrNoFirewallCmd) {
		t.Errorf("Remove without firewall-cmd → %v", err)
	}
}

func TestLinuxProvider_Reload(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("success\n", nil)
	if err := p.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "--reload" {
		t.Errorf("reload args: %v", (*calls)[0].args)
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("firewall-cmd: Error: COMMAND_FAILED"))
	if err := p.Reload(context.Background()); err == nil {
		t.Error("Reload should propagate a runner error")
	}
	// missing firewall-cmd binary
	if err := (&linuxProvider{bin: "", run: nil}).Reload(context.Background()); !errors.Is(err, ErrNoFirewallCmd) {
		t.Errorf("Reload without firewall-cmd → %v", err)
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

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}

func TestLinuxProvider_ListRichRules(t *testing.T) {
	t.Parallel()
	out := "rule family=\"ipv4\" service name=\"ssh\" accept\n\nrule family=\"ipv4\" source address=\"10.0.0.0/8\" drop\n"
	p, calls := newRecordingProvider(out, nil)
	rules, err := p.ListRichRules(context.Background(), "public")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("want 2 rules (blank line dropped), got %d: %v", len(rules), rules)
	}
	if strings.Join((*calls)[0].args, " ") != "--permanent --zone=public --list-rich-rules" {
		t.Errorf("list args: %v", (*calls)[0].args)
	}
	// missing firewall-cmd
	if _, err := (&linuxProvider{}).ListRichRules(context.Background(), "public"); !IsNoFirewallCmd(err) {
		t.Errorf("missing firewall-cmd → ErrNoFirewallCmd, got %v", err)
	}
	// firewall-cmd error propagates
	pErr, _ := newRecordingProvider("", errors.New("exit 1: bad zone"))
	if _, err := pErr.ListRichRules(context.Background(), "nope"); err == nil {
		t.Error("firewall-cmd error should propagate")
	}
}
