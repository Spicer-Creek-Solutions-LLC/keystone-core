// SPDX-License-Identifier: Apache-2.0

//go:build linux

package langpkg

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
	return &linuxProvider{pipBin: "pip", npmBin: "npm", gemBin: "gem", run: run}, &calls
}

// --- pip --------------------------------------------------------------

func TestLinuxProvider_HasPipPackage(t *testing.T) {
	t.Parallel()
	pipShow := "Name: requests\nVersion: 2.31.0\nSummary: …\n"
	p, calls := newRecordingProvider(pipShow, nil)
	has, ver, err := p.HasPipPackage(context.Background(), "requests")
	if err != nil || !has || ver != "2.31.0" {
		t.Fatalf("present: %v %q %v", has, ver, err)
	}
	if (*calls)[0].bin != "pip" || strings.Join((*calls)[0].args, " ") != "show requests" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// version line missing → installed with empty version
	p, _ = newRecordingProvider("Name: requests\n", nil)
	has, ver, _ = p.HasPipPackage(context.Background(), "requests")
	if !has || ver != "" {
		t.Errorf("no-version: %v %q", has, ver)
	}
	// non-zero exit → absent (no error)
	p, _ = newRecordingProvider("", errors.New("exit 1"))
	has, ver, err = p.HasPipPackage(context.Background(), "missing")
	if err != nil || has || ver != "" {
		t.Errorf("absent: %v %q %v", has, ver, err)
	}
	// missing pip
	p = &linuxProvider{}
	if _, _, err := p.HasPipPackage(context.Background(), "x"); !errors.Is(err, ErrNoPip) {
		t.Errorf("missing pip → %v", err)
	}
}

func TestLinuxProvider_PipInstallUninstall(t *testing.T) {
	t.Parallel()
	// install latest
	p, calls := newRecordingProvider("", nil)
	if err := p.InstallPipPackage(context.Background(), "requests", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install requests" {
		t.Errorf("install: %+v", (*calls)[0])
	}
	// install pinned
	p, calls = newRecordingProvider("", nil)
	if err := p.InstallPipPackage(context.Background(), "requests", "2.31.0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install requests==2.31.0" {
		t.Errorf("install pinned: %+v", (*calls)[0])
	}
	// uninstall
	p, calls = newRecordingProvider("", nil)
	if err := p.UninstallPipPackage(context.Background(), "requests"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "uninstall -y requests" {
		t.Errorf("uninstall: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("denied"))
	if err := p.InstallPipPackage(context.Background(), "x", ""); err == nil {
		t.Error("runner error should propagate")
	}
	// missing pip
	p = &linuxProvider{}
	if err := p.InstallPipPackage(context.Background(), "x", ""); !errors.Is(err, ErrNoPip) {
		t.Errorf("missing pip → %v", err)
	}
	if err := p.UninstallPipPackage(context.Background(), "x"); !errors.Is(err, ErrNoPip) {
		t.Errorf("missing pip uninstall → %v", err)
	}
}

// --- npm --------------------------------------------------------------

func TestLinuxProvider_HasNpmPackage(t *testing.T) {
	t.Parallel()
	npmJSON := `{"dependencies":{"pm2":{"version":"5.3.0"}}}`
	p, calls := newRecordingProvider(npmJSON, nil)
	has, ver, err := p.HasNpmPackage(context.Background(), "pm2")
	if err != nil || !has || ver != "5.3.0" {
		t.Fatalf("present: %v %q %v", has, ver, err)
	}
	if (*calls)[0].bin != "npm" || strings.Join((*calls)[0].args, " ") != "list -g pm2 --depth=0 --json" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// dependency map empty → absent
	p, _ = newRecordingProvider(`{"dependencies":{}}`, errors.New("exit 1"))
	has, _, _ = p.HasNpmPackage(context.Background(), "pm2")
	if has {
		t.Error("empty deps → absent")
	}
	// empty output → absent
	p, _ = newRecordingProvider("", nil)
	if has, _, _ := p.HasNpmPackage(context.Background(), "pm2"); has {
		t.Error("empty output → absent")
	}
	// non-JSON output → absent (defensive)
	p, _ = newRecordingProvider("npm ERR! something\n", errors.New("exit 1"))
	if has, _, _ := p.HasNpmPackage(context.Background(), "pm2"); has {
		t.Error("non-JSON → absent")
	}
	// missing npm
	p = &linuxProvider{}
	if _, _, err := p.HasNpmPackage(context.Background(), "x"); !errors.Is(err, ErrNoNpm) {
		t.Errorf("missing npm → %v", err)
	}
}

func TestLinuxProvider_NpmInstallUninstall(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.InstallNpmPackage(context.Background(), "pm2", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install -g pm2" {
		t.Errorf("install: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.InstallNpmPackage(context.Background(), "pm2", "5.3.0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install -g pm2@5.3.0" {
		t.Errorf("install pinned: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.UninstallNpmPackage(context.Background(), "pm2"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "uninstall -g pm2" {
		t.Errorf("uninstall: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.InstallNpmPackage(context.Background(), "x", ""); !errors.Is(err, ErrNoNpm) {
		t.Errorf("missing npm install → %v", err)
	}
	if err := p.UninstallNpmPackage(context.Background(), "x"); !errors.Is(err, ErrNoNpm) {
		t.Errorf("missing npm uninstall → %v", err)
	}
}

// --- gem --------------------------------------------------------------

func TestLinuxProvider_HasGemPackage(t *testing.T) {
	t.Parallel()
	// single version
	p, calls := newRecordingProvider("\nbundler (2.4.0)\n", nil)
	has, ver, err := p.HasGemPackage(context.Background(), "bundler")
	if err != nil || !has || ver != "2.4.0" {
		t.Fatalf("single: %v %q %v", has, ver, err)
	}
	if (*calls)[0].bin != "gem" || strings.Join((*calls)[0].args, " ") != "list --exact bundler" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// multiple versions — first is highest
	p, _ = newRecordingProvider("bundler (2.4.0, 2.3.0, 1.17.0)\n", nil)
	if has, ver, _ := p.HasGemPackage(context.Background(), "bundler"); !has || ver != "2.4.0" {
		t.Errorf("multi: %v %q", has, ver)
	}
	// no match — gem still exits 0 with "true" output convention; treat absent
	p, _ = newRecordingProvider("\n\n*** LOCAL GEMS ***\n\n", nil)
	if has, _, _ := p.HasGemPackage(context.Background(), "missing"); has {
		t.Error("no-match → absent")
	}
	// non-zero exit → absent
	p, _ = newRecordingProvider("", errors.New("exit 1"))
	if has, _, _ := p.HasGemPackage(context.Background(), "x"); has {
		t.Error("err → absent")
	}
	// missing gem
	p = &linuxProvider{}
	if _, _, err := p.HasGemPackage(context.Background(), "x"); !errors.Is(err, ErrNoGem) {
		t.Errorf("missing gem → %v", err)
	}
}

func TestLinuxProvider_GemInstallUninstall(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.InstallGemPackage(context.Background(), "bundler", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install bundler" {
		t.Errorf("install: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.InstallGemPackage(context.Background(), "bundler", "2.4.0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "install bundler -v 2.4.0" {
		t.Errorf("install pinned: %+v", (*calls)[0])
	}
	p, calls = newRecordingProvider("", nil)
	if err := p.UninstallGemPackage(context.Background(), "bundler"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "uninstall -aIx bundler" {
		t.Errorf("uninstall: %+v", (*calls)[0])
	}
	p = &linuxProvider{}
	if err := p.InstallGemPackage(context.Background(), "x", ""); !errors.Is(err, ErrNoGem) {
		t.Errorf("missing gem install → %v", err)
	}
	if err := p.UninstallGemPackage(context.Background(), "x"); !errors.Is(err, ErrNoGem) {
		t.Errorf("missing gem uninstall → %v", err)
	}
}

// --- exec + defaultProvider ------------------------------------------

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/pip", nil); err == nil {
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
