// SPDX-License-Identifier: Apache-2.0

package lvm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func TestDiffPVSets(t *testing.T) {
	t.Parallel()
	ref := func(paths ...string) []pvRef {
		out := make([]pvRef, len(paths))
		for i, p := range paths {
			out[i] = pvRef{orig: p, canon: p}
		}
		return out
	}
	cases := []struct {
		name             string
		want, live       []pvRef
		wantAdd, wantRem []string
	}{
		{"equal", ref("/dev/sdb", "/dev/sdc"), ref("/dev/sdb", "/dev/sdc"), nil, nil},
		{"add one", ref("/dev/sdb", "/dev/sdc"), ref("/dev/sdb"), []string{"/dev/sdc"}, nil},
		{"remove one", ref("/dev/sdb"), ref("/dev/sdb", "/dev/sdc"), nil, []string{"/dev/sdc"}},
		{"swap", ref("/dev/sdb", "/dev/sdd"), ref("/dev/sdb", "/dev/sdc"), []string{"/dev/sdd"}, []string{"/dev/sdc"}},
		{"dedup want", ref("/dev/sdb", "/dev/sdb"), ref(), []string{"/dev/sdb"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			add, rem := diffPVSets(c.want, c.live)
			if !reflect.DeepEqual(add, c.wantAdd) || !reflect.DeepEqual(rem, c.wantRem) {
				t.Errorf("add=%v rem=%v; want add=%v rem=%v", add, rem, c.wantAdd, c.wantRem)
			}
		})
	}
}

// diffPVSets matches on the canonical form: a by-id symlink and the
// device it resolves to are the same PV.
func TestDiffPVSets_Canonical(t *testing.T) {
	t.Parallel()
	want := []pvRef{{orig: "/dev/disk/by-id/wwn-x", canon: "/dev/sdb"}}
	live := []pvRef{{orig: "/dev/sdb", canon: "/dev/sdb"}}
	if add, rem := diffPVSets(want, live); add != nil || rem != nil {
		t.Errorf("symlink and canonical should match: add=%v rem=%v", add, rem)
	}
}

func vgDecl(state string, pvs ...string) *statemgmt.Declaration {
	anyPVs := make([]any, len(pvs))
	for i, p := range pvs {
		anyPVs[i] = p
	}
	return decl("vg0", state, map[string]any{"vg": "vg0", "pvs": anyPVs})
}

func liveVG(f *fakeProvider, pvs ...string) {
	f.vgs["vg0"] = true
	f.pvSets["vg0"] = append([]string(nil), pvs...)
}

func TestCheck_VGPVSet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// converged: declared == live
	f := newFake()
	liveVG(f, "/dev/sdb", "/dev/sdc")
	res, err := NewWithProvider(f).Check(ctx, vgDecl("present", "/dev/sdb", "/dev/sdc"))
	if err != nil || !res.Matches {
		t.Errorf("equal set → converged; got %+v err=%v", res, err)
	}

	// drift: live missing /dev/sdc
	f = newFake()
	liveVG(f, "/dev/sdb")
	res, _ = NewWithProvider(f).Check(ctx, vgDecl("present", "/dev/sdb", "/dev/sdc"))
	if res.Matches {
		t.Error("differing PV set → want drift")
	}
}

func TestApply_VGExtend(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb")
	m := NewWithProvider(f)

	sr, err := m.Apply(ctx, vgDecl("present", "/dev/sdb", "/dev/sdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.extendVGCalls) != 1 || !reflect.DeepEqual(f.extendVGCalls[0].PVs, []string{"/dev/sdc"}) {
		t.Fatalf("expected one vgextend of /dev/sdc: %+v", f.extendVGCalls)
	}
	if len(f.reduceVGCalls) != 0 {
		t.Errorf("no vgreduce expected: %+v", f.reduceVGCalls)
	}
	// idempotent: live now has both
	sr2, _ := m.Apply(ctx, vgDecl("present", "/dev/sdb", "/dev/sdc"))
	if sr2.Changed {
		t.Errorf("second apply should be a no-op: %+v", sr2)
	}
}

func TestApply_VGReduce(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb", "/dev/sdc")
	m := NewWithProvider(f)

	sr, err := m.Apply(ctx, vgDecl("present", "/dev/sdb"))
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed || len(f.reduceVGCalls) != 1 || !reflect.DeepEqual(f.reduceVGCalls[0].PVs, []string{"/dev/sdc"}) {
		t.Fatalf("expected one vgreduce of /dev/sdc: %+v", f.reduceVGCalls)
	}
	if len(f.extendVGCalls) != 0 {
		t.Errorf("no vgextend expected: %+v", f.extendVGCalls)
	}
}

func TestApply_VGExtendThenReduce(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb", "/dev/sdc")
	m := NewWithProvider(f)

	// swap sdc → sdd
	sr, err := m.Apply(ctx, vgDecl("present", "/dev/sdb", "/dev/sdd"))
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Fatal("expected change")
	}
	if len(f.extendVGCalls) != 1 || !reflect.DeepEqual(f.extendVGCalls[0].PVs, []string{"/dev/sdd"}) {
		t.Errorf("vgextend /dev/sdd: %+v", f.extendVGCalls)
	}
	if len(f.reduceVGCalls) != 1 || !reflect.DeepEqual(f.reduceVGCalls[0].PVs, []string{"/dev/sdc"}) {
		t.Errorf("vgreduce /dev/sdc: %+v", f.reduceVGCalls)
	}
	// converged after the swap
	if sr2, _ := m.Apply(ctx, vgDecl("present", "/dev/sdb", "/dev/sdd")); sr2.Changed {
		t.Errorf("second apply should be a no-op: %+v", sr2)
	}
}

func TestApply_VGPVSet_NoChange(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb", "/dev/sdc")
	sr, err := NewWithProvider(f).Apply(ctx, vgDecl("present", "/dev/sdb", "/dev/sdc"))
	if err != nil {
		t.Fatal(err)
	}
	if sr.Changed || len(f.extendVGCalls) != 0 || len(f.reduceVGCalls) != 0 {
		t.Errorf("equal set → no-op; got %+v ext=%v red=%v", sr, f.extendVGCalls, f.reduceVGCalls)
	}
}

func TestApply_VGCanonicalMatch(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb")
	f.canon = map[string]string{"/dev/disk/by-id/wwn-x": "/dev/sdb"}
	// declared by-id path resolves to the live canonical → no churn
	sr, err := NewWithProvider(f).Apply(ctx, vgDecl("present", "/dev/disk/by-id/wwn-x"))
	if err != nil {
		t.Fatal(err)
	}
	if sr.Changed || len(f.extendVGCalls) != 0 || len(f.reduceVGCalls) != 0 {
		t.Errorf("by-id == canonical → no-op; got %+v ext=%v red=%v", sr, f.extendVGCalls, f.reduceVGCalls)
	}
}

func TestApply_VGReduceError(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	liveVG(f, "/dev/sdb", "/dev/sdc")
	f.reduceVGErr = errors.New("vgreduce: in use")
	_, err := NewWithProvider(f).Apply(ctx, vgDecl("present", "/dev/sdb"))
	if err == nil {
		t.Error("vgreduce error should surface from Apply")
	}
}
