// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"errors"
	"testing"
)

// --- fakeAppArmorProvider ----------------------------------------------

type fakeAppArmorProvider struct {
	modes    map[string]string // profile → "enforce"/"complain"; absent = unloaded
	getErr   error
	setErr   error
	setCalls []aaSetCall
}

type aaSetCall struct{ Profile, Mode string }

func (f *fakeAppArmorProvider) GetProfileMode(_ context.Context, profile string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.modes[profile], nil
}

func (f *fakeAppArmorProvider) SetProfileMode(_ context.Context, profile, mode string) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.modes == nil {
		f.modes = map[string]string{}
	}
	if mode == AADisable {
		delete(f.modes, profile)
	} else {
		f.modes[profile] = mode
	}
	f.setCalls = append(f.setCalls, aaSetCall{profile, mode})
	return nil
}

// --- params ------------------------------------------------------------

func TestAppArmor_Parse(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Op != OpAppArmorProfile || p.AAProfile != "/usr/bin/foo" || p.AAMode != "enforce" {
		t.Errorf("parsed: %+v", p)
	}

	bad := []map[string]any{
		{"apparmor.profile": "/usr/bin/foo"},                                              // missing mode
		{"apparmor.profile_mode": "enforce"},                                              // mode without profile
		{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": 1},                  // non-string mode
		{"apparmor.profile": 1, "apparmor.profile_mode": "enforce"},                       // non-string profile
		{"mode": "enforcing", "apparmor.profile": "/usr/bin/foo"},                         // two ops
		{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "x", "value": "on"}, // value w/o boolean
	}
	for _, b := range bad {
		if _, err := parseParams(decl("l", b)); err == nil {
			t.Errorf("parseParams(%v) should error", b)
		}
	}
}

func TestAppArmor_Validate(t *testing.T) {
	t.Parallel()
	ok := []map[string]any{
		{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"},
		{"apparmor.profile": "usr.bin.foo", "apparmor.profile_mode": "complain"},
		{"apparmor.profile": "libreoffice-soffice", "apparmor.profile_mode": "disable"},
	}
	for _, params := range ok {
		if err := (&Module{}).Validate(decl("l", params)); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", params, err)
		}
	}
	bad := []map[string]any{
		{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "permissive"}, // not an AA mode
		{"apparmor.profile": "with space", "apparmor.profile_mode": "enforce"},      // bad profile chars
		{"apparmor.profile": "", "apparmor.profile_mode": "enforce"},                // empty profile
	}
	for _, params := range bad {
		if err := (&Module{}).Validate(decl("l", params)); err == nil {
			t.Errorf("Validate(%v) should error", params)
		}
	}
}

// --- module check / apply ----------------------------------------------

func aaModule(f *fakeAppArmorProvider) *Module {
	return NewWithProviders(nil, f).(*Module)
}

func TestAppArmor_Check_Converged(t *testing.T) {
	t.Parallel()
	f := &fakeAppArmorProvider{modes: map[string]string{"/usr/bin/foo": "enforce"}}
	res, err := aaModule(f).Check(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if err != nil || !res.Matches {
		t.Errorf("enforce==enforce → want match; got %+v %v", res, err)
	}
}

func TestAppArmor_Apply_Drift(t *testing.T) {
	t.Parallel()
	f := &fakeAppArmorProvider{modes: map[string]string{"/usr/bin/foo": "complain"}}
	res, err := aaModule(f).Apply(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || !res.Success {
		t.Errorf("complain→enforce should change: %+v", res)
	}
	if len(f.setCalls) != 1 || f.setCalls[0] != (aaSetCall{"/usr/bin/foo", "enforce"}) {
		t.Errorf("setCalls = %v", f.setCalls)
	}
	// idempotent re-apply
	res2, _ := aaModule(f).Apply(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if res2.Changed {
		t.Error("second apply should be a no-op")
	}
}

func TestAppArmor_Disable_UnloadsLoadedProfile(t *testing.T) {
	t.Parallel()
	f := &fakeAppArmorProvider{modes: map[string]string{"/usr/bin/foo": "enforce"}}
	res, err := aaModule(f).Apply(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "disable"}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || len(f.setCalls) != 1 || f.setCalls[0].Mode != "disable" {
		t.Errorf("loaded→disable should call aa-disable: %+v %v", res, f.setCalls)
	}
}

func TestAppArmor_Disable_AlreadyUnloadedIsConverged(t *testing.T) {
	t.Parallel()
	f := &fakeAppArmorProvider{modes: map[string]string{}} // not loaded
	res, err := aaModule(f).Apply(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/ghost", "apparmor.profile_mode": "disable"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || len(f.setCalls) != 0 {
		t.Errorf("absent + want disable → converged no-op; got %+v %v", res, f.setCalls)
	}
}

func TestAppArmor_Check_Drift_DiffShowsUnloaded(t *testing.T) {
	t.Parallel()
	f := &fakeAppArmorProvider{modes: map[string]string{}} // not loaded
	res, err := aaModule(f).Check(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Matches || !contains(res.Diff, "unloaded") || !contains(res.Diff, "enforce") {
		t.Errorf("want drift diff naming unloaded→enforce; got %+v", res)
	}
}

func TestAppArmor_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	getF := &fakeAppArmorProvider{getErr: errors.New("aa-status boom")}
	if _, err := aaModule(getF).Check(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"})); err == nil {
		t.Error("GetProfileMode error should propagate")
	}
	setF := &fakeAppArmorProvider{modes: map[string]string{"/usr/bin/foo": "complain"}, setErr: errors.New("aa-enforce boom")}
	res, err := aaModule(setF).Apply(context.Background(), decl("l", map[string]any{"apparmor.profile": "/usr/bin/foo", "apparmor.profile_mode": "enforce"}))
	if err == nil || res.Success {
		t.Errorf("SetProfileMode error should fail the apply; got %+v %v", res, err)
	}
}

func TestOpString_AppArmor(t *testing.T) {
	t.Parallel()
	if OpAppArmorProfile.String() != "apparmor.profile" {
		t.Errorf("OpString = %q", OpAppArmorProfile.String())
	}
}
