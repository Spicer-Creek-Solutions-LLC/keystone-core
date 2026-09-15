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
		sys := SearchPath(r, env(nil))
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
	got := SearchPath(RoleServer, env(map[string]string{"KEYSTONE_SERVER_CONFIG": "/tmp/x.toml"}))
	if len(got) != 1 || got[0] != "/tmp/x.toml" {
		t.Errorf("SearchPath = %v, want exactly the override", got)
	}
}

// Nothing reads the working directory. A configuration found there makes the
// same command behave differently depending on where it was run.
func TestNoRoleReadsTheWorkingDirectory(t *testing.T) {
	for _, r := range []Role{RoleOperator, RoleServer, RoleAgent} {
		for _, p := range SearchPath(r, env(map[string]string{"HOME": "/home/x"})) {
			if !filepath.IsAbs(p) {
				t.Errorf("%s searches a relative path %q", r, p)
			}
			if strings.HasPrefix(p, "./") || p == "config.toml" {
				t.Errorf("%s searches the working directory: %q", r, p)
			}
		}
	}
}

// Only the operator has a per-user file. A service reading a home directory
// reads from whichever account it happens to run as.
func TestOnlyTheOperatorHasAPerUserFile(t *testing.T) {
	e := env(map[string]string{"HOME": "/home/x"})
	if n := len(SearchPath(RoleOperator, e)); n != 2 {
		t.Errorf("operator search path has %d entries, want a per-user file and a system file", n)
	}
	for _, r := range []Role{RoleServer, RoleAgent} {
		for _, p := range SearchPath(r, e) {
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
