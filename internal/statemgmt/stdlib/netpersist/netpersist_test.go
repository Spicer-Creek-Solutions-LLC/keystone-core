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
