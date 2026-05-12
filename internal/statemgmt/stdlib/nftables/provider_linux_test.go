//go:build linux

package nftables

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	return &linuxProvider{bin: "nft", run: run}, &calls
}

const sampleChainDump = `table inet filter {
	chain input {
		type filter hook input priority filter; policy accept;
		ct state established,related accept # handle 4
		tcp dport 22 accept # handle 5
		tcp dport { 80, 443 } accept # handle 6
	}
}
`

func TestParseChainRules(t *testing.T) {
	t.Parallel()
	got := parseChainRules(sampleChainDump)
	want := []RuleHandle{
		{Text: "ct state established,related accept", Handle: 4},
		{Text: "tcp dport 22 accept", Handle: 5},
		{Text: "tcp dport { 80, 443 } accept", Handle: 6},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseChainRules =\n%#v\nwant\n%#v", got, want)
	}
	// a regular (non-base) chain with no `type … hook` line, and an
	// empty chain.
	plain := parseChainRules("table ip nat {\n\tchain custom {\n\t\tip daddr 1.2.3.4 drop # handle 9\n\t}\n}\n")
	if len(plain) != 1 || plain[0].Handle != 9 || plain[0].Text != "ip daddr 1.2.3.4 drop" {
		t.Errorf("plain chain: %#v", plain)
	}
	if r := parseChainRules("table inet filter {\n\tchain input {\n\t}\n}\n"); len(r) != 0 {
		t.Errorf("empty chain should yield no rules, got %#v", r)
	}
	if r := parseChainRules(""); len(r) != 0 {
		t.Errorf("empty dump should yield no rules, got %#v", r)
	}
}

func TestSplitHandle(t *testing.T) {
	t.Parallel()
	if txt, h, ok := splitHandle("tcp dport 22 accept # handle 5"); !ok || h != 5 || txt != "tcp dport 22 accept" {
		t.Errorf("splitHandle = %q,%d,%v", txt, h, ok)
	}
	if _, _, ok := splitHandle("type filter hook input priority filter; policy accept;"); ok {
		t.Error("a line with no handle should not split")
	}
	if _, _, ok := splitHandle("tcp dport 22 accept # handle notanint"); ok {
		t.Error("a non-integer handle should not split")
	}
}

func TestLinuxProvider_ListRuleHandles(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider(sampleChainDump, nil)
	got, err := p.ListRuleHandles(context.Background(), "inet", "filter", "input")
	if err != nil || len(got) != 3 {
		t.Fatalf("ListRuleHandles = %#v, %v", got, err)
	}
	if strings.Join((*calls)[0].args, " ") != "--handle list chain inet filter input" {
		t.Errorf("list args: %v", (*calls)[0].args)
	}
	// missing table/chain → (nil, nil)
	for _, msg := range []string{
		"nft --handle list chain inet filter nope: exit 1: Error: No such file or directory",
		"nft --handle list chain inet nope input: exit 1: Error: table 'nope' does not exist",
	} {
		p, _ := newRecordingProvider("", errors.New(msg))
		r, err := p.ListRuleHandles(context.Background(), "inet", "filter", "input")
		if err != nil || r != nil {
			t.Errorf("missing-object %q → %#v, %v", msg, r, err)
		}
	}
	// a real error propagates
	p, _ = newRecordingProvider("", errors.New("nft …: exit 1: Error: Could not process rule: Operation not permitted"))
	if _, err := p.ListRuleHandles(context.Background(), "inet", "filter", "input"); err == nil {
		t.Error("a non-missing error should propagate")
	}
	// missing nft binary
	if _, err := (&linuxProvider{bin: "", run: nil}).ListRuleHandles(context.Background(), "inet", "filter", "input"); !errors.Is(err, ErrNoNft) {
		t.Errorf("missing nft → %v", err)
	}
}

func TestLinuxProvider_AddDeleteArgs(t *testing.T) {
	t.Parallel()
	rule := []string{"tcp", "dport", "22", "accept"}

	// append
	p, calls := newRecordingProvider("", nil)
	if err := p.AddRule(context.Background(), "inet", "filter", "input", -1, rule); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "add rule inet filter input tcp dport 22 accept" {
		t.Errorf("append args: %v", (*calls)[0].args)
	}
	// insert at index, ip6
	p, calls = newRecordingProvider("", nil)
	if err := p.AddRule(context.Background(), "ip6", "filter", "forward", 0, []string{"drop"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "insert rule ip6 filter forward index 0 drop" {
		t.Errorf("insert args: %v", (*calls)[0].args)
	}
	// delete by handle
	p, calls = newRecordingProvider("", nil)
	if err := p.DeleteRule(context.Background(), "inet", "filter", "input", 5); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "delete rule inet filter input handle 5" {
		t.Errorf("delete args: %v", (*calls)[0].args)
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("nft: Error: No such file or directory"))
	if err := p.AddRule(context.Background(), "inet", "filter", "input", -1, rule); err == nil {
		t.Error("AddRule should propagate a runner error")
	}
	// missing nft binary
	noNft := &linuxProvider{bin: "", run: nil}
	if err := noNft.AddRule(context.Background(), "inet", "filter", "input", -1, rule); !errors.Is(err, ErrNoNft) {
		t.Errorf("AddRule without nft → %v", err)
	}
	if err := noNft.DeleteRule(context.Background(), "inet", "filter", "input", 1); !errors.Is(err, ErrNoNft) {
		t.Errorf("DeleteRule without nft → %v", err)
	}
}

func TestLinuxProvider_SaveRuleset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nftables.conf")
	p, calls := newRecordingProvider("table inet filter {\n}\n", nil)
	if err := p.SaveRuleset(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "list ruleset" {
		t.Errorf("Save args: %v", (*calls)[0].args)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "table inet filter") {
		t.Errorf("save file: %q %v", b, err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("save file mode = %o, want 0600", fi.Mode().Perm())
	}
	// existing file's mode is preserved
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	p, _ = newRecordingProvider("table inet filter {\n}\n", nil)
	if err := p.SaveRuleset(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
		t.Errorf("save should preserve existing mode, got %o", fi.Mode().Perm())
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("nft list ruleset: exit 1: Error: Operation not permitted"))
	if err := p.SaveRuleset(context.Background(), path); err == nil {
		t.Error("Save should propagate a runner error")
	}
	// missing nft binary
	if err := (&linuxProvider{bin: "", run: nil}).SaveRuleset(context.Background(), path); !errors.Is(err, ErrNoNft) {
		t.Errorf("Save without nft → %v", err)
	}
	// unwritable path
	p, _ = newRecordingProvider("data", nil)
	if err := p.SaveRuleset(context.Background(), filepath.Join(dir, "no-such-dir", "x")); err == nil {
		t.Error("Save to an unwritable path should error")
	}
}

func TestIsMissingObject(t *testing.T) {
	t.Parallel()
	if !isMissingObject(errors.New("Error: No such file or directory")) {
		t.Error("no-such-file should be a missing object")
	}
	if !isMissingObject(errors.New("Error: chain 'input' does not exist")) {
		t.Error("does-not-exist should be a missing object")
	}
	if isMissingObject(errors.New("Error: Operation not permitted")) {
		t.Error("EPERM is not a missing object")
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/nft", nil); err == nil {
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
