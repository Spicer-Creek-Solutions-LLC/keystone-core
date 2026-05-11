//go:build linux

package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ---- parseDpkgStatus ---------------------------------------------

func TestParseDpkgStatus_Installed(t *testing.T) {
	t.Parallel()
	// "ii" = desired-installed + current-installed; the abbrev's
	// second char is "i". A trailing newline is normal.
	out := "ii  1.18.0-6ubuntu14.4\n"
	info, err := parseDpkgStatus("nginx", out)
	if err != nil {
		t.Fatalf("parseDpkgStatus: %v", err)
	}
	if !info.Installed {
		t.Errorf("Installed = false, want true")
	}
	if info.Version != "1.18.0-6ubuntu14.4" {
		t.Errorf("Version = %q, want 1.18.0-6ubuntu14.4", info.Version)
	}
}

func TestParseDpkgStatus_NotInstalled(t *testing.T) {
	t.Parallel()
	// "un" = desired-unknown + current-not-installed.
	info, err := parseDpkgStatus("nginx", "un  \n")
	if err != nil {
		t.Fatalf("parseDpkgStatus: %v", err)
	}
	if info.Installed {
		t.Error("un status should be not-installed")
	}
	if info.Version != "" {
		t.Errorf("Version should be empty when not installed; got %q", info.Version)
	}
}

func TestParseDpkgStatus_ConfigFilesRemain(t *testing.T) {
	t.Parallel()
	// "rc" = remove-config-files; second char "c" → treated as
	// not-installed for state=installed comparison purposes.
	info, _ := parseDpkgStatus("nginx", "rc  1.18.0-6\n")
	if info.Installed {
		t.Error("rc status should NOT count as installed")
	}
}

func TestParseDpkgStatus_HalfInstalled(t *testing.T) {
	t.Parallel()
	// "iH" = desired-install + current-half-installed; second
	// char "H" → treated as not-installed.
	info, _ := parseDpkgStatus("nginx", "iH  1.18.0-6\n")
	if info.Installed {
		t.Error("half-installed should NOT count as installed")
	}
}

func TestParseDpkgStatus_EmptyOutput(t *testing.T) {
	t.Parallel()
	// dpkg-query exit 1 produces empty stdout.
	info, err := parseDpkgStatus("nginx", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Installed {
		t.Error("empty stdout should mean not installed")
	}
}

func TestParseDpkgStatus_MalformedAbbrev(t *testing.T) {
	t.Parallel()
	// 1-char status is malformed.
	_, err := parseDpkgStatus("nginx", "x\n")
	if err == nil {
		t.Error("expected error on malformed status")
	}
}

func TestParseDpkgStatus_UnknownStatus(t *testing.T) {
	t.Parallel()
	// 'Z' second char is invented; should surface as error so a
	// future dpkg format change is loud.
	_, err := parseDpkgStatus("nginx", "iZ  1.0\n")
	if err == nil {
		t.Error("expected error on unknown status code")
	}
}

func TestParseDpkgStatus_NoVersionField(t *testing.T) {
	t.Parallel()
	// "ii" without trailing version — uncommon but possible.
	info, _ := parseDpkgStatus("phantom", "ii\n")
	if !info.Installed {
		t.Error("ii alone should still report installed")
	}
	if info.Version != "" {
		t.Errorf("Version = %q, want empty", info.Version)
	}
}

// ---- aptProvider arg formation -----------------------------------

// capturingRunner records the args passed to the runner so tests
// can assert what would have been exec'd without invoking apt-get.
type capturingRunner struct {
	calls []capturedCall
	err   error
}

type capturedCall struct {
	Bin  string
	Args []string
	Env  []string
}

func (c *capturingRunner) run(_ context.Context, bin string, args []string, env []string) error {
	c.calls = append(c.calls, capturedCall{
		Bin:  bin,
		Args: append([]string(nil), args...),
		Env:  append([]string(nil), env...),
	})
	return c.err
}

func newAptForTest(r commandRunner, lookup dpkgLookupFn) *aptProvider {
	return &aptProvider{
		aptGet:     "/usr/bin/apt-get",
		dpkgQuery:  "/usr/bin/dpkg-query",
		runner:     r,
		dpkgLookup: lookup,
	}
}

func TestAptProvider_Install_NoVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newAptForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(cr.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(cr.calls))
	}
	got := cr.calls[0]
	wantArgs := []string{"install", "-y", "--no-install-recommends", "nginx"}
	if !sliceEq(got.Args, wantArgs) {
		t.Errorf("args = %v, want %v", got.Args, wantArgs)
	}
	if !containsEnv(got.Env, "DEBIAN_FRONTEND=noninteractive") {
		t.Errorf("env missing DEBIAN_FRONTEND: %v", got.Env)
	}
	if got.Bin != "/usr/bin/apt-get" {
		t.Errorf("Bin = %q", got.Bin)
	}
}

func TestAptProvider_Install_WithVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newAptForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", "1.20"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := cr.calls[0]
	if got.Args[len(got.Args)-1] != "nginx=1.20" {
		t.Errorf("pinned-version arg wrong; got %v", got.Args)
	}
}

func TestAptProvider_Remove(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newAptForTest(cr.run, nil)
	if err := p.Remove(context.Background(), "nginx"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	wantArgs := []string{"remove", "-y", "nginx"}
	if !sliceEq(cr.calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", cr.calls[0].Args, wantArgs)
	}
}

func TestAptProvider_RunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{err: errors.New("apt-get: held back")}
	p := newAptForTest(cr.run, nil)
	err := p.Install(context.Background(), "nginx", "")
	if err == nil || !strings.Contains(err.Error(), "held back") {
		t.Errorf("err = %v, want runner's underlying error", err)
	}
}

// ---- aptProvider Lookup via injected dpkgLookup ------------------

func TestAptProvider_Lookup_DispatchesToParser(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, name string) (string, int, error) {
		return "ii  1.20\n", 0, nil
	}
	p := newAptForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed || info.Version != "1.20" {
		t.Errorf("Lookup: %+v", info)
	}
}

func TestAptProvider_Lookup_NotInstalledExit1(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "", 1, errors.New("dpkg-query: no packages found")
	}
	p := newAptForTest(nil, fake)
	info, err := p.Lookup("ghost")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Installed {
		t.Error("exit 1 should mean not installed")
	}
}

func TestAptProvider_Lookup_OtherErrorSurfaces(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "", 127, errors.New("dpkg-query: command not found")
	}
	p := newAptForTest(nil, fake)
	_, err := p.Lookup("nginx")
	if err == nil {
		t.Fatal("expected error from non-1 exit")
	}
}

// ---- execRun + realDpkgLookup (binary-presence side) -------------

func TestExecRun_ExitError(t *testing.T) {
	t.Parallel()
	err := execRun(context.Background(), "/bin/false", nil, nil)
	if err == nil {
		t.Fatal("expected exit-1 error")
	}
	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("err = %v, want \"exit\" in message", err)
	}
}

func TestExecRun_BinaryNotFound(t *testing.T) {
	t.Parallel()
	err := execRun(context.Background(), "/no/such/bin", nil, nil)
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRealDpkgLookup_BinaryNotFound(t *testing.T) {
	t.Parallel()
	// Exercises the non-ExitError branch.
	_, _, err := realDpkgLookup(context.Background(), "/no/such/dpkg-query", "nginx")
	if err == nil {
		t.Fatal("expected lookup error")
	}
}

// ---- detect_linux + undetectedProvider ---------------------------

func TestUndetectedProvider_LookupAlwaysNotInstalled(t *testing.T) {
	t.Parallel()
	p := &undetectedProvider{}
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Installed {
		t.Error("undetectedProvider should report not installed")
	}
}

func TestUndetectedProvider_InstallNoBackend(t *testing.T) {
	t.Parallel()
	err := (&undetectedProvider{}).Install(context.Background(), "nginx", "")
	if !errors.Is(err, ErrNoBackend) {
		t.Errorf("err = %v, want ErrNoBackend", err)
	}
}

func TestUndetectedProvider_RemoveNoBackend(t *testing.T) {
	t.Parallel()
	err := (&undetectedProvider{}).Remove(context.Background(), "nginx")
	if !errors.Is(err, ErrNoBackend) {
		t.Errorf("err = %v, want ErrNoBackend", err)
	}
}

func TestDefaultProvider_PicksAptWhenAvailable(t *testing.T) {
	t.Parallel()
	// On a CI host with apt installed, defaultProvider returns an
	// aptProvider; on a CI host without apt, it returns
	// undetectedProvider. Both are valid outcomes — just assert
	// the result implements Provider.
	p := defaultProvider()
	if p == nil {
		t.Fatal("defaultProvider returned nil")
	}
}

// ---- helpers -----------------------------------------------------

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}
