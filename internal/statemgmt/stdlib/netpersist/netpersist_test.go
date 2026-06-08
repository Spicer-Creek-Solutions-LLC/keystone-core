// SPDX-License-Identifier: Apache-2.0

package netpersist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidBackend(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{Networkd, Netplan, Auto} {
		if !ValidBackend(ok) {
			t.Errorf("ValidBackend(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "nm", "ifupdown"} {
		if ValidBackend(bad) {
			t.Errorf("ValidBackend(%q) = true", bad)
		}
	}
}

func TestPaths(t *testing.T) {
	// Not parallel: mutates the package-level dirs.
	origND, origNP := NetworkdDir, NetplanDir
	NetworkdDir, NetplanDir = "/nd", "/np"
	defer func() { NetworkdDir, NetplanDir = origND, origNP }()

	if got := NetworkPath("eth0"); got != "/nd/10-kscore-eth0.network" {
		t.Errorf("NetworkPath = %q", got)
	}
	if got := NetdevPath("bond0"); got != "/nd/10-kscore-bond0.netdev" {
		t.Errorf("NetdevPath = %q", got)
	}
	if got := NetplanPath("eth0"); got != "/np/90-kscore-eth0.yaml" {
		t.Errorf("NetplanPath = %q", got)
	}
	if got := NetworkDropinPath("eth0", "route-10-0-0-0_24-main"); got != "/nd/10-kscore-eth0.network.d/route-10-0-0-0_24-main.conf" {
		t.Errorf("NetworkDropinPath = %q", got)
	}
	if got := NetplanRoutePath("10-0-0-0_24-main"); got != "/np/90-kscore-route-10-0-0-0_24-main.yaml" {
		t.Errorf("NetplanRoutePath = %q", got)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	origND := NetworkdDir
	NetworkdDir = dir
	defer func() { NetworkdDir = origND }()

	// removing an absent file is not an error
	if err := Remove(filepath.Join(dir, "nope")); err != nil {
		t.Errorf("Remove(absent) = %v, want nil", err)
	}
	// write then remove
	path := NetworkDropinPath("eth0", "r1")
	if err := Write(path, "[Route]\n"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(path); ok {
		t.Error("file should be gone after Remove")
	}
}

func TestReadWriteRemoveErrors(t *testing.T) {
	dir := t.TempDir()

	// Read of a directory is a non-ErrNotExist error.
	if _, _, err := Read(dir); err == nil {
		t.Error("Read of a directory should error")
	}
	// Write under a non-directory parent: mkdir fails.
	notDir := filepath.Join(dir, "afile")
	if err := os.WriteFile(notDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(notDir, "child.network"), "x"); err == nil {
		t.Error("Write under a non-directory parent should error")
	}
	// Remove of a non-empty directory is a non-ErrNotExist error.
	nonEmpty := filepath.Join(dir, "d", "inner")
	if err := os.MkdirAll(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Remove(filepath.Join(dir, "d")); err == nil {
		t.Error("Remove of a non-empty directory should error")
	}
}

func TestReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	origND := NetworkdDir
	NetworkdDir = filepath.Join(dir, "systemd-network")
	defer func() { NetworkdDir = origND }()

	path := NetworkPath("eth0")
	// absent
	if c, ok, err := Read(path); err != nil || ok || c != "" {
		t.Fatalf("absent = %q,%v,%v", c, ok, err)
	}
	// write (creates the dir) + read back
	if err := Write(path, "[Match]\nName=eth0\n"); err != nil {
		t.Fatal(err)
	}
	c, ok, err := Read(path)
	if err != nil || !ok || c != "[Match]\nName=eth0\n" {
		t.Errorf("round-trip = %q,%v,%v", c, ok, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s: %v", path, err)
	}
}

func TestDetectBackend(t *testing.T) {
	dir := t.TempDir()
	origNP := NetplanDir
	defer func() { NetplanDir = origNP }()

	NetplanDir = filepath.Join(dir, "no-netplan")
	if b, err := DetectBackend(); err != nil || b != Networkd {
		t.Errorf("no netplan dir → networkd; got %q,%v", b, err)
	}
	NetplanDir = filepath.Join(dir, "netplan")
	if err := os.MkdirAll(NetplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if b, err := DetectBackend(); err != nil || b != Netplan {
		t.Errorf("netplan dir present → netplan; got %q,%v", b, err)
	}
}
