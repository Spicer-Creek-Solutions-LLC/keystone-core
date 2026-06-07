// SPDX-License-Identifier: Apache-2.0

//go:build linux

package security

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const sampleAAStatus = `{
  "version": "1.1",
  "profiles": {
    "/usr/bin/foo": "enforce",
    "/usr/sbin/bar": "complain",
    "libreoffice-soffice": "unconfined"
  },
  "processes": {}
}`

func TestParseAAStatus(t *testing.T) {
	t.Parallel()
	profiles, err := parseAAStatus([]byte(sampleAAStatus))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"/usr/bin/foo":        "enforce",
		"/usr/sbin/bar":       "complain",
		"libreoffice-soffice": "unconfined",
	}
	if !reflect.DeepEqual(profiles, want) {
		t.Errorf("profiles = %v, want %v", profiles, want)
	}
	if _, err := parseAAStatus([]byte("not json")); err == nil {
		t.Error("invalid json should error")
	}
}

func TestLinuxAppArmor_GetProfileMode(t *testing.T) {
	t.Parallel()
	p := &linuxAppArmorProvider{
		statusBin: "/usr/sbin/aa-status",
		statusRun: func(context.Context) (string, error) { return sampleAAStatus, nil },
	}
	// loaded profile → its mode
	if mode, err := p.GetProfileMode(context.Background(), "/usr/bin/foo"); err != nil || mode != "enforce" {
		t.Errorf("foo = %q, %v; want enforce", mode, err)
	}
	// not-loaded profile → "" (the converged state for disable)
	if mode, err := p.GetProfileMode(context.Background(), "/usr/bin/ghost"); err != nil || mode != "" {
		t.Errorf("ghost = %q, %v; want empty", mode, err)
	}
}

func TestLinuxAppArmor_GetProfileMode_Unavailable(t *testing.T) {
	t.Parallel()
	// aa-status binary absent
	if _, err := (&linuxAppArmorProvider{}).GetProfileMode(context.Background(), "/usr/bin/foo"); !IsAppArmorUnavailable(err) {
		t.Errorf("missing aa-status → want ErrAppArmorUnavailable, got %v", err)
	}
	// status run fails
	p := &linuxAppArmorProvider{statusBin: "/usr/sbin/aa-status", statusRun: func(context.Context) (string, error) { return "", errors.New("not enabled") }}
	if _, err := p.GetProfileMode(context.Background(), "/usr/bin/foo"); !IsAppArmorUnavailable(err) {
		t.Errorf("status failure → want ErrAppArmorUnavailable, got %v", err)
	}
	// unparseable status
	p2 := &linuxAppArmorProvider{statusBin: "/usr/sbin/aa-status", statusRun: func(context.Context) (string, error) { return "garbage", nil }}
	if _, err := p2.GetProfileMode(context.Background(), "/usr/bin/foo"); !IsAppArmorUnavailable(err) {
		t.Errorf("bad json → want ErrAppArmorUnavailable, got %v", err)
	}
}

func TestLinuxAppArmor_SetProfileMode_ArgFormation(t *testing.T) {
	t.Parallel()
	var got struct {
		bin  string
		args []string
	}
	p := &linuxAppArmorProvider{
		enforceBin:  "/usr/sbin/aa-enforce",
		complainBin: "/usr/sbin/aa-complain",
		disableBin:  "/usr/sbin/aa-disable",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			got.bin, got.args = bin, args
			return "", nil
		},
	}
	for _, tc := range []struct{ mode, bin string }{
		{AAEnforce, "/usr/sbin/aa-enforce"},
		{AAComplain, "/usr/sbin/aa-complain"},
		{AADisable, "/usr/sbin/aa-disable"},
	} {
		if err := p.SetProfileMode(context.Background(), "/usr/bin/foo", tc.mode); err != nil {
			t.Fatalf("%s: %v", tc.mode, err)
		}
		if got.bin != tc.bin || !reflect.DeepEqual(got.args, []string{"/usr/bin/foo"}) {
			t.Errorf("%s → %s %v, want %s [/usr/bin/foo]", tc.mode, got.bin, got.args, tc.bin)
		}
	}
	// unknown mode
	if err := p.SetProfileMode(context.Background(), "/usr/bin/foo", "permissive"); err == nil {
		t.Error("unknown mode should error")
	}
	// missing tool binary
	if err := (&linuxAppArmorProvider{}).SetProfileMode(context.Background(), "/usr/bin/foo", AAEnforce); !IsAppArmorUnavailable(err) {
		t.Errorf("missing aa-enforce → want ErrAppArmorUnavailable, got %v", err)
	}
}

func TestDefaultAppArmorProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultAppArmorProvider() == nil {
		t.Fatal("nil AppArmor provider")
	}
}
