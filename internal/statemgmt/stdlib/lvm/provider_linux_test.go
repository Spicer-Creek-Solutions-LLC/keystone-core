// SPDX-License-Identifier: Apache-2.0

//go:build linux

package lvm

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
	p := &linuxProvider{bins: map[string]string{}, run: run}
	for _, name := range lvmTools {
		p.bins[name] = name
	}
	return p, &calls
}

// --- has --------------------------------------------------------------

func TestLinuxProvider_HasPV(t *testing.T) {
	t.Parallel()
	// non-empty trimmed output → present
	p, calls := newRecordingProvider("  /dev/sdb1\n", nil)
	has, err := p.HasPV(context.Background(), "/dev/sdb1")
	if err != nil || !has {
		t.Fatalf("present: %v %v", has, err)
	}
	if (*calls)[0].bin != "pvs" || strings.Join((*calls)[0].args, " ") != "--noheadings -o pv_name /dev/sdb1" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// empty output → absent
	p, _ = newRecordingProvider("\n", nil)
	has, err = p.HasPV(context.Background(), "/dev/sdb1")
	if err != nil || has {
		t.Errorf("empty: %v %v", has, err)
	}
	// runner error → absent (no error)
	p, _ = newRecordingProvider("", errors.New("Failed to find physical volume"))
	has, err = p.HasPV(context.Background(), "/dev/sdb1")
	if err != nil || has {
		t.Errorf("err: %v %v", has, err)
	}
	// missing binary
	p = &linuxProvider{bins: map[string]string{}}
	if _, err := p.HasPV(context.Background(), "/dev/sdb1"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing pvs → %v", err)
	}
}

func TestLinuxProvider_HasVG(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("  myvg\n", nil)
	has, _ := p.HasVG(context.Background(), "myvg")
	if !has || strings.Join((*calls)[0].args, " ") != "--noheadings -o vg_name myvg" {
		t.Errorf("vgs: %+v %v", (*calls)[0], has)
	}
	p, _ = newRecordingProvider("", errors.New("VG not found"))
	if has, _ := p.HasVG(context.Background(), "x"); has {
		t.Error("err → absent")
	}
}

func TestLinuxProvider_HasLV(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("  home\n", nil)
	has, _ := p.HasLV(context.Background(), "myvg", "home")
	if !has || strings.Join((*calls)[0].args, " ") != "--noheadings -o lv_name myvg/home" {
		t.Errorf("lvs: %+v %v", (*calls)[0], has)
	}
	p, _ = newRecordingProvider("", errors.New("not found"))
	if has, _ := p.HasLV(context.Background(), "myvg", "home"); has {
		t.Error("err → absent")
	}
}

// --- create/remove ---------------------------------------------------

func TestLinuxProvider_PVCreateRemove(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.CreatePV(context.Background(), "/dev/sdb1"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "pvcreate" || strings.Join((*calls)[0].args, " ") != "/dev/sdb1" {
		t.Errorf("create: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.RemovePV(context.Background(), "/dev/sdb1"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "pvremove" || strings.Join((*calls)[0].args, " ") != "/dev/sdb1" {
		t.Errorf("remove: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("not safe"))
	if err := p.CreatePV(context.Background(), "/dev/sdb1"); err == nil {
		t.Error("create runner error should propagate")
	}
	// missing binary
	p = &linuxProvider{bins: map[string]string{}}
	if err := p.CreatePV(context.Background(), "/dev/sdb1"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing pvcreate → %v", err)
	}
	if err := p.RemovePV(context.Background(), "/dev/sdb1"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing pvremove → %v", err)
	}
}

func TestLinuxProvider_VGCreateRemove(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.CreateVG(context.Background(), "myvg", []string{"/dev/sdb1", "/dev/sdc1"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "vgcreate" || strings.Join((*calls)[0].args, " ") != "myvg /dev/sdb1 /dev/sdc1" {
		t.Errorf("create: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.RemoveVG(context.Background(), "myvg"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-y myvg" {
		t.Errorf("remove: %+v", (*calls)[0])
	}
	p = &linuxProvider{bins: map[string]string{}}
	if err := p.CreateVG(context.Background(), "x", []string{"/dev/sda"}); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing vgcreate → %v", err)
	}
	if err := p.RemoveVG(context.Background(), "x"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing vgremove → %v", err)
	}
}

func TestLinuxProvider_GetVGPVs(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("  /dev/sdb\n  /dev/sdc\n\n", nil)
	pvs, err := p.GetVGPVs(context.Background(), "myvg")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "--noheadings -o pv_name myvg" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	if strings.Join(pvs, ",") != "/dev/sdb,/dev/sdc" {
		t.Errorf("parsed PVs: %v", pvs)
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("vgs boom"))
	if _, err := p.GetVGPVs(context.Background(), "myvg"); err == nil {
		t.Error("GetVGPVs runner error should propagate")
	}
	// missing binary
	p = &linuxProvider{bins: map[string]string{}}
	if _, err := p.GetVGPVs(context.Background(), "x"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing vgs → %v", err)
	}
}

func TestLinuxProvider_VGExtendReduce(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.ExtendVG(context.Background(), "myvg", []string{"/dev/sdc", "/dev/sdd"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "vgextend" || strings.Join((*calls)[0].args, " ") != "myvg /dev/sdc /dev/sdd" {
		t.Errorf("vgextend: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.ReduceVG(context.Background(), "myvg", []string{"/dev/sdc"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "vgreduce" || strings.Join((*calls)[0].args, " ") != "myvg /dev/sdc" {
		t.Errorf("vgreduce: %+v", (*calls)[0])
	}
	// runner errors propagate
	p, _ = newRecordingProvider("", errors.New("still in use"))
	if err := p.ReduceVG(context.Background(), "myvg", []string{"/dev/sdc"}); err == nil {
		t.Error("vgreduce runner error should propagate")
	}
	// missing binaries
	p = &linuxProvider{bins: map[string]string{}}
	if err := p.ExtendVG(context.Background(), "x", []string{"/dev/sda"}); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing vgextend → %v", err)
	}
	if err := p.ReduceVG(context.Background(), "x", []string{"/dev/sda"}); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing vgreduce → %v", err)
	}
}

func TestLinuxProvider_Canonicalize(t *testing.T) {
	t.Parallel()
	p, _ := newRecordingProvider("", nil)
	// a real, non-symlinked path resolves to itself
	got, err := p.Canonicalize(context.Background(), "/dev/null")
	if err != nil || got != "/dev/null" {
		t.Errorf("Canonicalize(/dev/null) = %q,%v", got, err)
	}
	// an unresolvable path is returned unchanged (best-effort)
	got, err = p.Canonicalize(context.Background(), "/dev/does-not-exist-xyz")
	if err != nil || got != "/dev/does-not-exist-xyz" {
		t.Errorf("Canonicalize(absent) = %q,%v; want input unchanged", got, err)
	}
}

func TestLinuxProvider_LVCreateSize(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.CreateLV(context.Background(), "myvg", "home", "10G", ""); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "lvcreate" || strings.Join((*calls)[0].args, " ") != "-y -n home -L 10G myvg" {
		t.Errorf("create-size: %+v", (*calls)[0])
	}
}

func TestLinuxProvider_LVCreateExtents(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.CreateLV(context.Background(), "myvg", "data", "", "100%FREE"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-y -n data -l 100%FREE myvg" {
		t.Errorf("create-extents: %+v", (*calls)[0])
	}
}

func TestLinuxProvider_LVCreateNeitherSizeNorExtents(t *testing.T) {
	t.Parallel()
	p, _ := newRecordingProvider("", nil)
	if err := p.CreateLV(context.Background(), "myvg", "h", "", ""); err == nil {
		t.Error("create without size or extents should error")
	}
}

func TestLinuxProvider_LVRemove(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.RemoveLV(context.Background(), "myvg", "home"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-y myvg/home" {
		t.Errorf("remove: %+v", (*calls)[0])
	}
	p = &linuxProvider{bins: map[string]string{}}
	if err := p.CreateLV(context.Background(), "x", "y", "1G", ""); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing lvcreate → %v", err)
	}
	if err := p.RemoveLV(context.Background(), "x", "y"); !errors.Is(err, ErrNoLVM) {
		t.Errorf("missing lvremove → %v", err)
	}
}

func TestLinuxProvider_RunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	p, _ := newRecordingProvider("", errors.New("permission denied"))
	if err := p.CreatePV(context.Background(), "/dev/sdb1"); err == nil {
		t.Error("CreatePV runner error should propagate")
	}
	if err := p.CreateVG(context.Background(), "x", []string{"/dev/sda"}); err == nil {
		t.Error("CreateVG runner error should propagate")
	}
	if err := p.CreateLV(context.Background(), "x", "y", "1G", ""); err == nil {
		t.Error("CreateLV runner error should propagate")
	}
	if err := p.RemoveLV(context.Background(), "x", "y"); err == nil {
		t.Error("RemoveLV runner error should propagate")
	}
}

// --- exec + defaultProvider ------------------------------------------

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/lvs", nil); err == nil {
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
