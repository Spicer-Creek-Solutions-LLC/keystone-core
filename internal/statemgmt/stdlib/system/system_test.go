package system

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "system:" + name,
		Module: "system",
		State:  state,
		Name:   name,
		Params: params,
	}
}

// --- fakeProvider -----------------------------------------------------

type fakeProvider struct {
	banners       map[string]string
	readBanErr    error
	writeBanErr   error
	rebootNeeded  bool
	rebootNeedErr error
	schedErr      error
	locale        string
	readLocErr    error
	writeLocErr   error

	bannerWrites []bannerWrite
	scheduled    []int
	localeSets   []string
}

type bannerWrite struct {
	Name    string
	Content string
}

func newFake() *fakeProvider {
	return &fakeProvider{banners: map[string]string{}}
}

func (f *fakeProvider) ReadBanner(_ context.Context, name string) (string, error) {
	if f.readBanErr != nil {
		return "", f.readBanErr
	}
	return f.banners[name], nil
}
func (f *fakeProvider) WriteBanner(_ context.Context, name, content string) error {
	if f.writeBanErr != nil {
		return f.writeBanErr
	}
	f.banners[name] = content
	f.bannerWrites = append(f.bannerWrites, bannerWrite{Name: name, Content: content})
	return nil
}
func (f *fakeProvider) IsRebootNeeded(_ context.Context, _ string) (bool, error) {
	return f.rebootNeeded, f.rebootNeedErr
}
func (f *fakeProvider) ScheduleReboot(_ context.Context, delay int) error {
	if f.schedErr != nil {
		return f.schedErr
	}
	f.scheduled = append(f.scheduled, delay)
	f.rebootNeeded = false
	return nil
}
func (f *fakeProvider) ReadLocale(_ context.Context) (string, error) {
	return f.locale, f.readLocErr
}
func (f *fakeProvider) WriteLocale(_ context.Context, lang string) error {
	if f.writeLocErr != nil {
		return f.writeLocErr
	}
	f.locale = lang
	f.localeSets = append(f.localeSets, lang)
	return nil
}

// --- params / validate -----------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banners": "motd"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_ExactlyOneOp(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("no op should error")
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x", "reboot": true})); err == nil {
		t.Error("banner+reboot should error")
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x", "locale": "en_US.UTF-8"})); err == nil {
		t.Error("banner+locale should error")
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true, "locale": "C"})); err == nil {
		t.Error("reboot+locale should error")
	}
}

func TestParse_BannerOp(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "Welcome\n"}))
	if err != nil || p.Op != OpBanner || p.BannerName != "motd" || p.BannerContent != "Welcome\n" {
		t.Errorf("parse: %+v %v", p, err)
	}
	// content key required
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd"})); err == nil {
		t.Error("banner without content should error")
	}
	// reboot params rejected
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x", "when_file": "/var/run/x"})); err == nil {
		t.Error("when_file with banner should error")
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x", "delay": 1})); err == nil {
		t.Error("delay with banner should error")
	}
	// non-string types
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": 1, "content": "x"})); err == nil {
		t.Error("non-string banner should error")
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"banner": "motd", "content": 1})); err == nil {
		t.Error("non-string content should error")
	}
}

func TestParse_RebootOp(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true}))
	if err != nil || p.Op != OpReboot || p.WhenFile != defaultWhenFile || p.Delay != defaultDelay {
		t.Errorf("defaults: %+v %v", p, err)
	}
	// custom when_file + delay (incl. delay coercion forms)
	for _, v := range []any{5, int64(5), float64(5), "5"} {
		p, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true, "when_file": "/run/reboot", "delay": v}))
		if err != nil || p.Delay != 5 || p.WhenFile != "/run/reboot" {
			t.Errorf("delay=%v: %+v %v", v, p, err)
		}
	}
	// reboot: false rejected
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": false})); err == nil {
		t.Error("reboot:false should be rejected")
	}
	// banner/locale params rejected
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true, "content": "x"})); err == nil {
		t.Error("content with reboot should error")
	}
	// non-bool reboot
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": "true"})); err == nil {
		t.Error("non-bool reboot should error")
	}
	// bad delay forms
	for _, v := range []any{1.5, "x", true, []any{}} {
		if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true, "delay": v})); err == nil {
			t.Errorf("delay=%v should error", v)
		}
	}
	// non-string when_file
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"reboot": true, "when_file": 1})); err == nil {
		t.Error("non-string when_file should error")
	}
}

func TestParse_LocaleOp(t *testing.T) {
	t.Parallel()
	p, err := parseParams(decl("l", StatePresent, map[string]any{"locale": "en_US.UTF-8"}))
	if err != nil || p.Op != OpLocale || p.Locale != "en_US.UTF-8" {
		t.Errorf("parse: %+v %v", p, err)
	}
	// banner/reboot params rejected
	for _, k := range []string{"content", "when_file", "delay"} {
		var v any = "x"
		if k == "delay" {
			v = 1
		}
		if _, err := parseParams(decl("l", StatePresent, map[string]any{"locale": "C", k: v})); err == nil {
			t.Errorf("%s with locale should error", k)
		}
	}
	if _, err := parseParams(decl("l", StatePresent, map[string]any{"locale": 1})); err == nil {
		t.Error("non-string locale should error")
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"banner motd ok", decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x"}), false},
		{"banner issue ok", decl("l", StatePresent, map[string]any{"banner": "issue", "content": ""}), false},
		{"banner issue_net ok", decl("l", StatePresent, map[string]any{"banner": "issue_net", "content": "x"}), false},
		{"banner absent ok", decl("l", StateAbsent, map[string]any{"banner": "motd", "content": "ignored"}), false},
		{"bad banner name", decl("l", StatePresent, map[string]any{"banner": "frob", "content": "x"}), true},
		{"reboot defaults ok", decl("l", StatePresent, map[string]any{"reboot": true}), false},
		{"reboot delay 60", decl("l", StatePresent, map[string]any{"reboot": true, "delay": 60}), false},
		{"reboot delay 0", decl("l", StatePresent, map[string]any{"reboot": true, "delay": 0}), false},
		{"reboot delay 61", decl("l", StatePresent, map[string]any{"reboot": true, "delay": 61}), true},
		{"reboot delay -1", decl("l", StatePresent, map[string]any{"reboot": true, "delay": -1}), true},
		{"reboot relative when_file", decl("l", StatePresent, map[string]any{"reboot": true, "when_file": "reboot-required"}), true},
		{"reboot empty when_file", decl("l", StatePresent, map[string]any{"reboot": true, "when_file": ""}), true},
		{"reboot absent rejected", decl("l", StateAbsent, map[string]any{"reboot": true}), true},
		{"locale UTF-8 ok", decl("l", StatePresent, map[string]any{"locale": "en_US.UTF-8"}), false},
		{"locale C ok", decl("l", StatePresent, map[string]any{"locale": "C"}), false},
		{"locale POSIX ok", decl("l", StatePresent, map[string]any{"locale": "POSIX"}), false},
		{"locale modifier ok", decl("l", StatePresent, map[string]any{"locale": "de_DE.UTF-8@euro"}), false},
		{"bad locale", decl("l", StatePresent, map[string]any{"locale": "english"}), true},
		{"locale absent rejected", decl("l", StateAbsent, map[string]any{"locale": "C"}), true},
		{"bad state", decl("l", "frob", map[string]any{"banner": "motd", "content": "x"}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestOpString(t *testing.T) {
	t.Parallel()
	if OpBanner.String() != "banner" || OpReboot.String() != "reboot" || OpLocale.String() != "locale" || OpUnknown.String() != "unknown" {
		t.Error("Op.String mapping wrong")
	}
}

// --- banner op -------------------------------------------------------

func TestBanner_PresentDrift(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.banners["motd"] = "old\n"
	m := NewWithProvider(f)
	d := decl("motd", StatePresent, map[string]any{"banner": "motd", "content": "new\n"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("expected drift, got %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.bannerWrites) != 1 || f.bannerWrites[0].Content != "new\n" {
		t.Errorf("writes: %+v", f.bannerWrites)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Errorf("converged apply should be no-op: %+v", sr)
	}
}

func TestBanner_AbsentClears(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.banners["issue"] = "old content\n"
	m := NewWithProvider(f)
	d := decl("blank", StateAbsent, map[string]any{"banner": "issue", "content": "ignored"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Errorf("expected drift (non-empty → absent), got %+v", r)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if f.banners["issue"] != "" {
		t.Errorf("banner should be blank, got %q", f.banners["issue"])
	}
	// already absent (empty)
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("empty file + state=absent should match: %+v", r)
	}
}

func TestBanner_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{readBanErr: errors.New("read")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x"})); err == nil {
		t.Error("read error should propagate")
	}
	f := newFake()
	f.writeBanErr = errors.New("write")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x"})); err == nil {
		t.Error("write error should propagate")
	}
}

// --- reboot op -------------------------------------------------------

func TestReboot_NotNeeded(t *testing.T) {
	t.Parallel()
	f := newFake() // rebootNeeded=false
	m := NewWithProvider(f)
	d := decl("r", StatePresent, map[string]any{"reboot": true})
	r, _ := m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("no marker → should match, got %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if sr.Changed || len(f.scheduled) != 0 {
		t.Errorf("no-op apply: %+v scheduled=%v", sr, f.scheduled)
	}
}

func TestReboot_Schedules(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.rebootNeeded = true
	m := NewWithProvider(f)
	d := decl("r", StatePresent, map[string]any{"reboot": true})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("marker present → should drift")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.scheduled) != 1 || f.scheduled[0] != defaultDelay {
		t.Errorf("scheduled: %+v", f.scheduled)
	}
	if !contains(sr.Comment, "1 minute") {
		t.Errorf("comment: %q", sr.Comment)
	}
}

func TestReboot_DelayZeroAndAboveOne(t *testing.T) {
	t.Parallel()
	// delay 0
	f := newFake()
	f.rebootNeeded = true
	m := NewWithProvider(f)
	d := decl("r", StatePresent, map[string]any{"reboot": true, "delay": 0})
	sr, _ := m.Apply(context.Background(), d)
	if f.scheduled[0] != 0 || !contains(sr.Comment, "now") || !contains(sr.Comment, "kernel kill") {
		t.Errorf("delay=0 sched=%v comment=%q", f.scheduled, sr.Comment)
	}
	// delay 5
	f = newFake()
	f.rebootNeeded = true
	m = NewWithProvider(f)
	d = decl("r", StatePresent, map[string]any{"reboot": true, "delay": 5})
	sr, _ = m.Apply(context.Background(), d)
	if f.scheduled[0] != 5 || !contains(sr.Comment, "5 minutes") {
		t.Errorf("delay=5 sched=%v comment=%q", f.scheduled, sr.Comment)
	}
}

func TestReboot_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{rebootNeedErr: errors.New("stat")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"reboot": true})); err == nil {
		t.Error("stat error should propagate")
	}
	f := newFake()
	f.rebootNeeded = true
	f.schedErr = errors.New("shutdown failed")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"reboot": true})); err == nil {
		t.Error("schedule error should propagate")
	}
}

// --- locale op -------------------------------------------------------

func TestLocale_Drift(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.locale = "en_GB.UTF-8"
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"locale": "en_US.UTF-8"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches || !contains(r.Diff, "en_GB.UTF-8") || !contains(r.Diff, "en_US.UTF-8") {
		t.Errorf("check: %+v", r)
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil || !sr.Changed {
		t.Fatalf("%+v %v", sr, err)
	}
	if len(f.localeSets) != 1 || f.localeSets[0] != "en_US.UTF-8" {
		t.Errorf("localeSets: %+v", f.localeSets)
	}
	// converged
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Errorf("converged check: %+v", r)
	}
}

func TestLocale_UnsetDisplayed(t *testing.T) {
	t.Parallel()
	f := newFake() // empty
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"locale": "C"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches || !contains(r.Diff, "<unset>") {
		t.Errorf("unset diff: %+v", r)
	}
}

func TestLocale_ErrorsPropagate(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(&fakeProvider{readLocErr: errors.New("read")})
	if _, err := m.Check(context.Background(), decl("l", StatePresent, map[string]any{"locale": "C"})); err == nil {
		t.Error("read error should propagate")
	}
	f := newFake()
	f.writeLocErr = errors.New("write")
	if _, err := NewWithProvider(f).Apply(context.Background(), decl("l", StatePresent, map[string]any{"locale": "C"})); err == nil {
		t.Error("write error should propagate")
	}
}

// --- module surface --------------------------------------------------

func TestParseError_FromCheckAndApply(t *testing.T) {
	t.Parallel()
	m := NewWithProvider(newFake())
	bad := decl("l", StatePresent, map[string]any{}) // no op
	if _, err := m.Check(context.Background(), bad); err == nil {
		t.Error("Check should reject an invalid declaration")
	}
	if _, err := m.Apply(context.Background(), bad); err == nil {
		t.Error("Apply should reject an invalid declaration")
	}
}

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "system" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("system should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	// banner LOW
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x"}), nil) != statemgmt.DriftSeverityLow {
		t.Error("banner → LOW")
	}
	// reboot HIGH
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"reboot": true}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("reboot → HIGH")
	}
	// locale LOW
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"locale": "C"}), nil) != statemgmt.DriftSeverityLow {
		t.Error("locale → LOW")
	}
	// unparseable → MEDIUM
	if dsm.DriftSeverity(decl("l", StatePresent, map[string]any{"banners": "x"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("unparseable → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("l", StatePresent, map[string]any{"banner": "motd", "content": "x"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("l", StatePresent, map[string]any{})); err == nil {
		t.Error("missing op should be rejected")
	}
}

func TestTest_Method(t *testing.T) {
	t.Parallel()
	f := newFake() // locale unset
	m := NewWithProvider(f)
	d := decl("l", StatePresent, map[string]any{"locale": "C"})
	if ok, err := m.Test(context.Background(), d); err != nil || ok {
		t.Errorf("Test before apply: ok=%v err=%v", ok, err)
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if ok, err := m.Test(context.Background(), d); err != nil || !ok {
		t.Errorf("Test after apply: ok=%v err=%v", ok, err)
	}
}

func TestSentinelMatchers(t *testing.T) {
	t.Parallel()
	if !IsUnsupportedOS(ErrUnsupportedOS) || IsUnsupportedOS(errors.New("x")) {
		t.Error("IsUnsupportedOS")
	}
	if !IsNoShutdown(ErrNoShutdown) || IsNoShutdown(errors.New("x")) {
		t.Error("IsNoShutdown")
	}
}

// --- helper ----------------------------------------------------------

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
