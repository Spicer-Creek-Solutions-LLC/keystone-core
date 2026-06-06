// SPDX-License-Identifier: Apache-2.0

//go:build linux

package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeLookPath resolves only the binaries listed in present.
func fakeLookPath(present ...string) func(string) (string, error) {
	set := make(map[string]struct{}, len(present))
	for _, p := range present {
		set[p] = struct{}{}
	}
	return func(bin string) (string, error) {
		if _, ok := set[bin]; ok {
			return "/usr/bin/" + bin, nil
		}
		return "", errors.New("not found")
	}
}

func TestResolveRebootDetector(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		present  []string
		wantBin  string
		wantArgs []string
		wantOK   bool
	}{
		{
			name:     "needs-restarting preferred over dnf",
			present:  []string{"needs-restarting", "dnf"},
			wantBin:  "/usr/bin/needs-restarting",
			wantArgs: []string{"-r"},
			wantOK:   true,
		},
		{
			name:     "dnf fallback",
			present:  []string{"dnf"},
			wantBin:  "/usr/bin/dnf",
			wantArgs: []string{"needs-restarting", "-r"},
			wantOK:   true,
		},
		{
			name:    "neither present",
			present: nil,
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bin, args, ok := resolveRebootDetector(fakeLookPath(tt.present...))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if bin != tt.wantBin || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("got %s %v, want %s %v", bin, args, tt.wantBin, tt.wantArgs)
			}
		})
	}
}

func TestDetectRebootProbe_NilWhenAbsent(t *testing.T) {
	t.Parallel()
	if detectRebootProbe(fakeLookPath()) != nil {
		t.Error("want nil probe when no detector binary is present")
	}
	if detectRebootProbe(fakeLookPath("needs-restarting")) == nil {
		t.Error("want non-nil probe when needs-restarting is present")
	}
}

// TestRealRebootProbe exercises the exit-code mapping against real
// binaries (/bin/sh -c 'exit N') so there is no written-then-exec'd
// stub to race on ETXTBSY.
func TestRealRebootProbe(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		bin      string
		args     []string
		wantNeed bool
		wantErr  bool
	}{
		{"exit 0 → not needed", "/bin/sh", []string{"-c", "exit 0"}, false, false},
		{"exit 1 → needed", "/bin/sh", []string{"-c", "exit 1"}, true, false},
		{"exit 2 → error", "/bin/sh", []string{"-c", "exit 2"}, false, true},
		{"missing binary → error", "/no/such/needs-restarting", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			need, err := realRebootProbe(tt.bin, tt.args)(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if need != tt.wantNeed {
				t.Errorf("need = %v, want %v", need, tt.wantNeed)
			}
		})
	}
}

func TestIsRebootNeeded_DetectorPath(t *testing.T) {
	t.Parallel()
	absentMarker := func(t *testing.T) string {
		return filepath.Join(t.TempDir(), "reboot-required") // never created
	}

	t.Run("marker absent + detector says needed", func(t *testing.T) {
		t.Parallel()
		p := &linuxProvider{rebootDetect: func(context.Context) (bool, error) { return true, nil }}
		need, err := p.IsRebootNeeded(context.Background(), absentMarker(t))
		if err != nil || !need {
			t.Errorf("need=%v err=%v, want true,nil", need, err)
		}
	})

	t.Run("marker absent + detector says not needed", func(t *testing.T) {
		t.Parallel()
		p := &linuxProvider{rebootDetect: func(context.Context) (bool, error) { return false, nil }}
		need, err := p.IsRebootNeeded(context.Background(), absentMarker(t))
		if err != nil || need {
			t.Errorf("need=%v err=%v, want false,nil", need, err)
		}
	})

	t.Run("marker absent + detector errors", func(t *testing.T) {
		t.Parallel()
		probeErr := errors.New("needs-restarting blew up")
		p := &linuxProvider{rebootDetect: func(context.Context) (bool, error) { return false, probeErr }}
		_, err := p.IsRebootNeeded(context.Background(), absentMarker(t))
		if !errors.Is(err, probeErr) {
			t.Errorf("err = %v, want the probe error to surface", err)
		}
	})

	t.Run("marker present short-circuits the detector", func(t *testing.T) {
		t.Parallel()
		called := false
		p := &linuxProvider{rebootDetect: func(context.Context) (bool, error) { called = true; return false, nil }}
		marker := filepath.Join(t.TempDir(), "reboot-required")
		if err := os.WriteFile(marker, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		need, err := p.IsRebootNeeded(context.Background(), marker)
		if err != nil || !need {
			t.Errorf("need=%v err=%v, want true,nil", need, err)
		}
		if called {
			t.Error("detector must not run when the marker is present")
		}
	})

	t.Run("marker absent + nil detector → not needed", func(t *testing.T) {
		t.Parallel()
		p := &linuxProvider{} // rebootDetect nil (Debian/Alpine)
		need, err := p.IsRebootNeeded(context.Background(), absentMarker(t))
		if err != nil || need {
			t.Errorf("need=%v err=%v, want false,nil", need, err)
		}
	})
}
