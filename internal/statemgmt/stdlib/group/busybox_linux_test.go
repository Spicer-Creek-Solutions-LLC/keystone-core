// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recorder struct {
	calls []recordedCall
	err   error
}

type recordedCall struct {
	bin  string
	args []string
}

func (r *recorder) run(_ context.Context, bin string, args []string) error {
	r.calls = append(r.calls, recordedCall{bin: bin, args: args})
	return r.err
}

func newTestBusybox(rec *recorder) *busyboxProvider {
	return &busyboxProvider{addgroup: "addgroup", delgroup: "delgroup", run: rec.run}
}

func TestBusybox_Add(t *testing.T) {
	t.Parallel()
	gid := 64030
	tests := []struct {
		name   string
		gid    *int
		system bool
		want   []string
	}{
		{"name only", nil, false, []string{"web"}},
		{"with gid", &gid, false, []string{"-g", "64030", "web"}},
		{"system", nil, true, []string{"-S", "web"}},
		{"gid + system", &gid, true, []string{"-g", "64030", "-S", "web"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			p := newTestBusybox(rec)
			if err := p.Add(context.Background(), "web", tt.gid, tt.system); err != nil {
				t.Fatalf("Add: %v", err)
			}
			if got := rec.calls[0]; got.bin != "addgroup" || !reflect.DeepEqual(got.args, tt.want) {
				t.Errorf("addgroup args = %v (bin %s), want %v", got.args, got.bin, tt.want)
			}
		})
	}
}

func TestBusybox_Mod_Unsupported(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	p := newTestBusybox(rec)
	err := p.Mod(context.Background(), "web", 1234)
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
	rec := &recorder{}
	p := newTestBusybox(rec)
	if err := p.Del(context.Background(), "web"); err != nil {
		t.Fatalf("Del: %v", err)
	}
	if got := rec.calls[0]; got.bin != "delgroup" || !reflect.DeepEqual(got.args, []string{"web"}) {
		t.Errorf("delgroup args = %v (bin %s), want [web]", got.args, got.bin)
	}
}

func TestBusybox_Add_ErrorPropagates(t *testing.T) {
	t.Parallel()
	rec := &recorder{err: errors.New("boom")}
	p := newTestBusybox(rec)
	if err := p.Add(context.Background(), "web", nil, false); err == nil {
		t.Fatal("expected the addgroup error to propagate")
	}
}
