//go:build linux

package at

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordedCall struct {
	args  []string
	stdin string
}

func recorder(out string, err error) (commandRunner, *[]recordedCall) {
	var calls []recordedCall
	run := func(_ context.Context, _ string, args []string, stdin string) (string, error) {
		calls = append(calls, recordedCall{args: args, stdin: stdin})
		return out, err
	}
	return run, &calls
}

func TestParseAtq(t *testing.T) {
	t.Parallel()
	// realistic `at -l` output (whitespace-padded)
	out := "3\tWed Jun  1 09:00:00 2026 a sbutts\n" +
		"7\tThu Jun  2 14:30:00 2026 b root\n" +
		"\n" +
		"warning: something\n"
	got := parseAtq(out)
	want := []string{"3", "7"}
	if len(got) != len(want) {
		t.Fatalf("parseAtq → %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseAtq[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(parseAtq("")) != 0 {
		t.Error("empty output should yield no ids")
	}
	if len(parseAtq("no jobs here\n")) != 0 {
		t.Error("a non-numeric line should be ignored")
	}
}

func TestIsDigits(t *testing.T) {
	t.Parallel()
	if !isDigits("12345") || isDigits("") || isDigits("12a") || isDigits("-1") {
		t.Error("isDigits")
	}
}

func TestLinuxProvider_ListJobs(t *testing.T) {
	run, calls := recorder("5\tFri Jun  3 10:00:00 2026 a u\n", nil)
	p := &linuxProvider{at: "at", run: run}
	ids, err := p.ListJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "5" {
		t.Errorf("ListJobs = %v", ids)
	}
	if strings.Join((*calls)[0].args, " ") != "-l" {
		t.Errorf("ListJobs args = %v", (*calls)[0].args)
	}

	// error propagates
	run, _ = recorder("", errors.New("at: cannot connect"))
	p = &linuxProvider{at: "at", run: run}
	if _, err := p.ListJobs(context.Background()); err == nil {
		t.Error("ListJobs should propagate a runner error")
	}
}

func TestLinuxProvider_JobScriptRemoveSubmit(t *testing.T) {
	run, calls := recorder("the script body\n", nil)
	p := &linuxProvider{at: "at", run: run}

	s, err := p.JobScript(context.Background(), "7")
	if err != nil || s != "the script body\n" {
		t.Fatalf("JobScript = %q,%v", s, err)
	}
	if strings.Join((*calls)[0].args, " ") != "-c 7" {
		t.Errorf("JobScript args = %v", (*calls)[0].args)
	}

	if err := p.Remove(context.Background(), "7"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[1].args, " ") != "-r 7" {
		t.Errorf("Remove args = %v", (*calls)[1].args)
	}

	if err := p.Submit(context.Background(), "b", "now + 1 hour", "# keystone-at: j\n/bin/x\n"); err != nil {
		t.Fatal(err)
	}
	submit := (*calls)[2]
	if strings.Join(submit.args, " ") != "-q b now + 1 hour" {
		t.Errorf("Submit args = %v", submit.args)
	}
	if submit.stdin != "# keystone-at: j\n/bin/x\n" {
		t.Errorf("Submit stdin = %q", submit.stdin)
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil, ""); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/at", nil, ""); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "cat", nil, "piped\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "piped\n" {
		t.Errorf("cat round-trip = %q", out)
	}
}

func TestNoAtProvider(t *testing.T) {
	t.Parallel()
	p := &noAtProvider{}
	if ids, err := p.ListJobs(context.Background()); err != nil || len(ids) != 0 {
		t.Errorf("ListJobs: %v %v", ids, err)
	}
	if _, err := p.JobScript(context.Background(), "1"); !errors.Is(err, ErrNoAt) {
		t.Errorf("JobScript err = %v", err)
	}
	if err := p.Submit(context.Background(), "a", "now", "x"); !errors.Is(err, ErrNoAt) {
		t.Errorf("Submit err = %v", err)
	}
	if err := p.Remove(context.Background(), "1"); !errors.Is(err, ErrNoAt) {
		t.Errorf("Remove err = %v", err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
