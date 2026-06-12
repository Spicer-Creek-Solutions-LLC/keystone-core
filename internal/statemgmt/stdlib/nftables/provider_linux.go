// SPDX-License-Identifier: Apache-2.0

//go:build linux

package nftables

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func defaultProvider() Provider {
	bin, _ := exec.LookPath("nft")
	return &linuxProvider{bin: bin, run: execRun}
}

type linuxProvider struct {
	bin string // resolved `nft` path ("" if absent)
	run commandRunner
}

func (p *linuxProvider) nft() (string, error) {
	if p.bin == "" {
		return "", ErrNoNft
	}
	return p.bin, nil
}

func (p *linuxProvider) ListRuleHandles(ctx context.Context, family, table, chain string) ([]RuleHandle, error) {
	bin, err := p.nft()
	if err != nil {
		return nil, err
	}
	out, err := p.run(ctx, bin, []string{"--handle", "list", "chain", family, table, chain})
	if err != nil {
		if isMissingObject(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseChainRules(out), nil
}

func (p *linuxProvider) AddRule(ctx context.Context, family, table, chain string, index int, rule []string) error {
	bin, err := p.nft()
	if err != nil {
		return err
	}
	var args []string
	if index < 0 {
		args = append([]string{"add", "rule", family, table, chain}, rule...)
	} else {
		args = append([]string{"insert", "rule", family, table, chain, "index", strconv.Itoa(index)}, rule...)
	}
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) DeleteRule(ctx context.Context, family, table, chain string, handle int) error {
	bin, err := p.nft()
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"delete", "rule", family, table, chain, "handle", strconv.Itoa(handle)})
	return err
}

func (p *linuxProvider) SaveRuleset(ctx context.Context, path string) error {
	bin, err := p.nft()
	if err != nil {
		return err
	}
	out, err := p.run(ctx, bin, []string{"list", "ruleset"})
	if err != nil {
		return fmt.Errorf("nft list ruleset: %w", err)
	}
	mode := os.FileMode(0o600)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(out), mode); err != nil { //nolint:gosec // operator-supplied save path; 0600 (firewall rules) or the existing file's mode
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// isMissingObject reports whether a `nft list chain` failure means the
// table or chain simply does not exist (as opposed to a real error).
func isMissingObject(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such file or directory") || strings.Contains(s, "does not exist")
}

// parseChainRules extracts the rule lines of a `nft --handle list
// chain` dump. Output looks like:
//
//	table inet filter {
//		chain input {
//			type filter hook input priority filter; policy accept;
//			ct state established,related accept # handle 4
//			tcp dport 22 accept # handle 5
//		}
//	}
//
// The chain-spec line (`type … hook …`) carries no handle and is
// skipped; every other line of the chain body that carries a
// `# handle N` comment is a rule.
func parseChainRules(out string) []RuleHandle {
	var rules []RuleHandle
	inChain := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !inChain {
			// The chain-opening line is `chain <name> {`, but `nft
			// --handle list chain` annotates it with a handle comment
			// (`chain input { # handle 1`); strip that before the suffix
			// check, or the chain is never entered and no rules parse.
			header := t
			if txt, _, ok := splitHandle(t); ok {
				header = txt
			}
			if strings.HasPrefix(header, "chain ") && strings.HasSuffix(header, "{") {
				inChain = true
			}
			continue
		}
		if t == "}" {
			break
		}
		text, handle, ok := splitHandle(t)
		if !ok {
			continue // chain-spec / policy line — no handle
		}
		if strings.HasPrefix(text, "type ") && strings.Contains(text, "hook ") {
			continue
		}
		rules = append(rules, RuleHandle{Text: text, Handle: handle})
	}
	return rules
}

// splitHandle splits a rule line "EXPR # handle N" into ("EXPR", N).
// ok is false if the line has no "# handle N" suffix or N is not an
// integer.
func splitHandle(line string) (string, int, bool) {
	const marker = " # handle "
	i := strings.LastIndex(line, marker)
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[i+len(marker):]))
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(line[:i]), n, true
}

// execRun is the production commandRunner. Captures combined output so
// nft's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed nft verbs + a validated family/table/chain + the operator-supplied rule spec from a validated state declaration
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
