// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

// recorder captures the (bin, args) of each commandRunner call so the
// tests assert BusyBox arg formation without invoking adduser.
type recorder struct {
	calls []recordedCall
	err   error // returned from each call (nil = success)
}

type recordedCall struct {
	bin  string
	args []string
}

func (r *recorder) run(_ context.Context, bin string, args []string) error {
	r.calls = append(r.calls, recordedCall{bin: bin, args: args})
	return r.err
}

// newTestBusybox builds a busyboxProvider wired to the recorder and a
// deterministic group-name resolver (gid N → "gidN") so tests stay
// hermetic.
func newTestBusybox(rec *recorder) *busyboxProvider {
	return &busyboxProvider{
		adduser:        "adduser",
		deluser:        "deluser",
		addgroup:       "addgroup",
		delgroup:       "delgroup",
		run:            rec.run,
		lookupFn:       func(string) (*UserInfo, error) { return nil, nil },
		groupNameByGID: func(gid int) (string, error) { return "gid" + strconv.Itoa(gid), nil },
	}
}

func TestBusybox_Add_MinimalFlags(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	if err := p.Add(context.Background(), AddOptions{Name: "alice"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 call, got %d: %+v", len(rec.calls), rec.calls)
	}
	// -D (no password) and -H (no home, since CreateHome=false) are
	// the non-negotiable defaults; the name is the final positional.
	want := []string{"-D", "-H", "alice"}
	if got := rec.calls[0]; got.bin != "adduser" || !reflect.DeepEqual(got.args, want) {
		t.Errorf("adduser args = %v (bin %s), want %v", got.args, got.bin, want)
	}
}

func TestBusybox_Add_AllScalarFlags(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	uid := 4242
	err := p.Add(context.Background(), AddOptions{
		Name:       "svc",
		UID:        &uid,
		Group:      "web",
		Home:       "/var/lib/svc",
		Shell:      "/sbin/nologin",
		Comment:    "service acct",
		System:     true,
		CreateHome: true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	want := []string{"-D", "-S", "-u", "4242", "-h", "/var/lib/svc", "-s", "/sbin/nologin", "-g", "service acct", "-G", "web", "svc"}
	if got := rec.calls[0].args; !reflect.DeepEqual(got, want) {
		t.Errorf("adduser args = %v, want %v", got, want)
	}
}

func TestBusybox_Add_PrimaryGIDResolvesToName(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	gid := 100
	if err := p.Add(context.Background(), AddOptions{Name: "bob", GID: &gid}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// -G takes a name; the numeric GID is resolved via groupNameByGID
	// (the test resolver maps 100 → "gid100").
	want := []string{"-D", "-H", "-G", "gid100", "bob"}
	if got := rec.calls[0].args; !reflect.DeepEqual(got, want) {
		t.Errorf("adduser args = %v, want %v", got, want)
	}
}

func TestBusybox_Add_PrimaryGIDResolveFails(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	p.groupNameByGID = func(int) (string, error) { return "", errors.New("no such gid") }
	gid := 999
	err := p.Add(context.Background(), AddOptions{Name: "bob", GID: &gid})
	if err == nil {
		t.Fatal("expected error when primary gid can't be resolved")
	}
	if len(rec.calls) != 0 {
		t.Errorf("adduser should not run when gid resolution fails; got %+v", rec.calls)
	}
}

func TestBusybox_Add_SupplementaryGroups(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	err := p.Add(context.Background(), AddOptions{Name: "alice", Groups: []string{"wheel", "docker"}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// adduser, then one `addgroup USER GROUP` per supplementary group.
	if len(rec.calls) != 3 {
		t.Fatalf("want 3 calls (adduser + 2 addgroup), got %d: %+v", len(rec.calls), rec.calls)
	}
	for i, want := range []recordedCall{
		{"adduser", []string{"-D", "-H", "alice"}},
		{"addgroup", []string{"alice", "wheel"}},
		{"addgroup", []string{"alice", "docker"}},
	} {
		if got := rec.calls[i]; got.bin != want.bin || !reflect.DeepEqual(got.args, want.args) {
			t.Errorf("call[%d] = %s %v, want %s %v", i, got.bin, got.args, want.bin, want.args)
		}
	}
}

func TestBusybox_Add_AdduserErrorStopsBeforeGroups(t *testing.T) {
	t.Parallel()
	rec := &recorder{err: errors.New("boom")}
	p := newTestBusybox(rec)
	err := p.Add(context.Background(), AddOptions{Name: "alice", Groups: []string{"wheel"}})
	if err == nil {
		t.Fatal("expected the adduser error to propagate")
	}
	// adduser failed → the addgroup loop must not run.
	if len(rec.calls) != 1 {
		t.Errorf("want 1 call (failed adduser only), got %d: %+v", len(rec.calls), rec.calls)
	}
}

func TestBusybox_Mod_Unsupported(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	err := p.Mod(context.Background(), ModOptions{Name: "alice", Shell: "/bin/sh"})
	if !errors.Is(err, ErrModUnsupported) {
		t.Errorf("Mod err = %v, want ErrModUnsupported", err)
	}
	if !IsModUnsupported(err) {
		t.Error("IsModUnsupported should match the Mod error")
	}
	if len(rec.calls) != 0 {
		t.Errorf("Mod must not run any command; got %+v", rec.calls)
	}
}

func TestBusybox_Del(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		removeHome bool
		want       []string
	}{
		{"keep home", false, []string{"alice"}},
		{"remove home", true, []string{"--remove-home", "alice"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			p := newTestBusybox(rec)
			if err := p.Del(context.Background(), "alice", tt.removeHome); err != nil {
				t.Fatalf("Del: %v", err)
			}
			if got := rec.calls[0]; got.bin != "deluser" || !reflect.DeepEqual(got.args, tt.want) {
				t.Errorf("deluser args = %v, want %v", got.args, tt.want)
			}
		})
	}
}

func TestBusybox_SetGroups_Diff(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	// Live: primary "users" (gid 100) + supplementary {wheel, audio}.
	// Desired supplementary: {wheel, docker}. So: add docker, remove
	// audio; wheel unchanged; the primary "users" is never removed
	// even though it isn't in the desired set.
	p.lookupFn = func(string) (*UserInfo, error) {
		return &UserInfo{Name: "alice", GID: 100, Groups: []string{"audio", "users", "wheel"}}, nil
	}
	p.groupNameByGID = func(int) (string, error) { return "users", nil }
	if err := p.SetGroups(context.Background(), "alice", []string{"wheel", "docker"}); err != nil {
		t.Fatalf("SetGroups: %v", err)
	}
	wantCalls := []recordedCall{
		{"addgroup", []string{"alice", "docker"}},
		{"delgroup", []string{"alice", "audio"}},
	}
	if !reflect.DeepEqual(rec.calls, wantCalls) {
		t.Errorf("calls = %+v, want %+v", rec.calls, wantCalls)
	}
}

func TestBusybox_SetGroups_LookupError(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	p.lookupFn = func(string) (*UserInfo, error) { return nil, errors.New("nss down") }
	if err := p.SetGroups(context.Background(), "alice", []string{"wheel"}); err == nil {
		t.Fatal("expected the lookup error to propagate")
	}
}

func TestGroupDelta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		current, desired []string
		wantAdd, wantRem []string
	}{
		{"empty both", nil, nil, nil, nil},
		{"all add", nil, []string{"b", "a"}, []string{"a", "b"}, nil},
		{"all remove", []string{"b", "a"}, nil, nil, []string{"a", "b"}},
		{"mixed", []string{"keep", "drop"}, []string{"keep", "new"}, []string{"new"}, []string{"drop"}},
		{"identical", []string{"a", "b"}, []string{"b", "a"}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAdd, gotRem := groupDelta(tt.current, tt.desired)
			if !reflect.DeepEqual(gotAdd, tt.wantAdd) {
				t.Errorf("toAdd = %v, want %v", gotAdd, tt.wantAdd)
			}
			if !reflect.DeepEqual(gotRem, tt.wantRem) {
				t.Errorf("toRemove = %v, want %v", gotRem, tt.wantRem)
			}
		})
	}
}

func TestStripGroup(t *testing.T) {
	t.Parallel()
	if got := stripGroup([]string{"a", "b"}, ""); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("blank drop should be a no-op, got %v", got)
	}
	if got := stripGroup([]string{"a", "primary", "b"}, "primary"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("stripGroup = %v, want [a b]", got)
	}
}
