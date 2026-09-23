package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// Each role reads its own file. A shared path would make the separation of the
// server's credentials from the agent's a matter of discipline.
func TestEachRoleHasItsOwnSystemPathAndOverride(t *testing.T) {
	seenPath, seenEnv := map[string]Role{}, map[string]Role{}
	for _, r := range []Role{RoleOperator, RoleServer, RoleAgent} {
		sys, err := SearchPath(r, env(nil))
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		last := sys[len(sys)-1]
		if other, dup := seenPath[last]; dup {
			t.Errorf("%s and %s share the system path %s", r, other, last)
		}
		seenPath[last] = r

		e := EnvOverride(r)
		if e == "" {
			t.Errorf("%s has no environment override", r)
		}
		if other, dup := seenEnv[e]; dup {
			t.Errorf("%s and %s share the override %s", r, other, e)
		}
		seenEnv[e] = r
	}
}

// The override displaces the search path rather than being prepended to it: a
// deployment that names a file must not silently fall through to another.
func TestOverrideReplacesTheSearchPath(t *testing.T) {
	got, err := SearchPath(RoleServer, env(map[string]string{"KEYSTONE_SERVER_CONFIG": "/tmp/x.toml"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "/tmp/x.toml" {
		t.Errorf("SearchPath = %v, want exactly the override", got)
	}
}

// Nothing reads the working directory. A configuration found there makes the
// same command behave differently depending on where it was run.
func TestNoRoleReadsTheWorkingDirectory(t *testing.T) {
	for _, r := range []Role{RoleOperator, RoleServer, RoleAgent} {
		got, err := SearchPath(r, env(map[string]string{"HOME": "/home/x"}))
		if err != nil {
			t.Fatalf("%s: %v", r, err)
		}
		for _, p := range got {
			if !filepath.IsAbs(p) {
				t.Errorf("%s searches a relative path %q", r, p)
			}
		}
	}
}

// The default paths were never the risk. Review of #331 found that an
// environment override was returned verbatim, so KEYSTONE_AGENT_CONFIG=x.toml
// made the agent read from wherever it happened to be started -- and the test
// above passed throughout, because it only exercised the defaults. A check whose
// name claims more than it covers is worse than no check.
func TestARelativeValueFromTheEnvironmentIsRefused(t *testing.T) {
	relatives := []string{"config.toml", "./config.toml", "../etc/keystone.toml", "sub/dir/c.toml"}

	for _, r := range []Role{RoleOperator, RoleServer, RoleAgent} {
		for _, rel := range relatives {
			e := env(map[string]string{EnvOverride(r): rel})

			if _, err := SearchPath(r, e); !errors.Is(err, ErrRelativePath) {
				t.Errorf("%s: SearchPath accepted %s=%q, err = %v", r, EnvOverride(r), rel, err)
			}
			if _, err := Locate(r, e); !errors.Is(err, ErrRelativePath) {
				t.Errorf("%s: Locate accepted %s=%q, err = %v", r, EnvOverride(r), rel, err)
			}
			// Refusal, never a silent fall-through to the next candidate: that
			// would read a file the operator did not name.
			if _, err := Locate(r, e); errors.Is(err, ErrNotConfigured) {
				t.Errorf("%s: a relative %s fell through to the system path", r, EnvOverride(r))
			}
		}
	}

	// A relative HOME or XDG_CONFIG_HOME builds a relative candidate just as an
	// override does, and only the operator role reads them.
	for _, v := range []string{"XDG_CONFIG_HOME", "HOME"} {
		if _, err := SearchPath(RoleOperator, env(map[string]string{v: "relative"})); !errors.Is(err, ErrRelativePath) {
			t.Errorf("SearchPath accepted %s=%q, err = %v", v, "relative", err)
		}
	}
}

// The error names the variable, so an operator with several set can fix the
// right one.
func TestTheRefusalNamesTheVariable(t *testing.T) {
	_, err := SearchPath(RoleServer, env(map[string]string{"KEYSTONE_SERVER_CONFIG": "c.toml"}))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "KEYSTONE_SERVER_CONFIG") {
		t.Errorf("error %q does not name the variable that supplied the path", err)
	}
}

// Only the operator has a per-user file. A service reading a home directory
// reads from whichever account it happens to run as.
func TestOnlyTheOperatorHasAPerUserFile(t *testing.T) {
	e := env(map[string]string{"HOME": "/home/x"})
	op, err := SearchPath(RoleOperator, e)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(op); n != 2 {
		t.Errorf("operator search path has %d entries, want a per-user file and a system file", n)
	}
	for _, r := range []Role{RoleServer, RoleAgent} {
		svc, err := SearchPath(r, e)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range svc {
			if strings.HasPrefix(p, "/home/") {
				t.Errorf("%s reads a home directory: %q", r, p)
			}
		}
	}
}

// Absence is an error, never a default. ADR-0009 § 1 and RFC 0003 both require
// a stated value rather than an invented one.
func TestAbsentConfigurationIsAnErrorRatherThanADefault(t *testing.T) {
	dir := t.TempDir()
	_, err := Locate(RoleAgent, env(map[string]string{"KEYSTONE_AGENT_CONFIG": filepath.Join(dir, "absent.toml")}))
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Locate returned %v, want ErrNotConfigured", err)
	}
}

func TestLocateReturnsTheFirstExistingPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "server.toml")
	if err := os.WriteFile(p, []byte("# empty\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Locate(RoleServer, env(map[string]string{"KEYSTONE_SERVER_CONFIG": p}))
	if err != nil || got != p {
		t.Fatalf("Locate = (%q, %v), want (%q, nil)", got, err, p)
	}
}

// A directory at a configuration path is not a configuration file.
func TestADirectoryIsNotConfiguration(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Locate(RoleOperator, env(map[string]string{"KEYSTONE_CONFIG": sub})); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Locate accepted a directory, err = %v", err)
	}
}

func TestLoadServerDefaultsAndFrozenSurface(t *testing.T) {
	p := filepath.Join(t.TempDir(), "server.toml")
	data := "[operator]\nadmin_group = \"keystone-admin\"\n\n[store]\npath = \"/var/lib/keystone/server.db\"\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadServer(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Operator.AdminGroup != "keystone-admin" || c.Store.Path != "/var/lib/keystone/server.db" {
		t.Fatalf("unexpected parsed configuration: %+v", c)
	}
	if c.Operator.MaxUnauthorizedConnections != DefaultMaxUnauthorizedConnections ||
		c.Operator.AuthorizationTimeout != DefaultAuthorizationTimeout ||
		c.Operator.MaxAuthorizedConnections != DefaultMaxAuthorizedConnections ||
		c.Operator.MaxFrameBytes != DefaultMaxFrameBytes {
		t.Fatalf("unexpected defaults: %+v", c.Operator)
	}
}

func TestLoadServerRefusesMissingGroupAndInvalidLimits(t *testing.T) {
	for name, data := range map[string]string{
		"missing group": "[operator]\nmax_frame_bytes = 1\n[store]\npath = \"/tmp/x\"\n",
		"zero limit":    "[operator]\nadmin_group = \"x\"\nmax_frame_bytes = 0\n[store]\npath = \"/tmp/x\"\n",
		"unknown key":   "[operator]\nadmin_group = \"x\"\nsurprise = 1\n[store]\npath = \"/tmp/x\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "server.toml")
			if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadServer(p); err == nil {
				t.Fatal("invalid server configuration was accepted")
			}
		})
	}
}
