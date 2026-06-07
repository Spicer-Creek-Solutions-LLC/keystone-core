// SPDX-License-Identifier: Apache-2.0

//go:build linux

package network

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistPath(t *testing.T) {
	t.Parallel()
	nd, err := persistPath(PersistNetworkd, "eth0")
	if err != nil || nd != filepath.Join(networkdDir, "10-kscore-eth0.network") {
		t.Errorf("networkd path = %q, %v", nd, err)
	}
	np, err := persistPath(PersistNetplan, "eth0")
	if err != nil || np != filepath.Join(netplanDir, "90-kscore-eth0.yaml") {
		t.Errorf("netplan path = %q, %v", np, err)
	}
	if _, err := persistPath("nm", "eth0"); err == nil {
		t.Error("unknown backend should error")
	}
}

func TestLinuxProvider_PersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Point the package-level dirs at the tempdir for this test.
	origND, origNP := networkdDir, netplanDir
	networkdDir = filepath.Join(dir, "systemd-network")
	netplanDir = filepath.Join(dir, "netplan")
	defer func() { networkdDir, netplanDir = origND, origNP }()

	p := &linuxProvider{}
	ctx := context.Background()

	// absent → (.., false, nil)
	if c, ok, err := p.GetPersisted(ctx, PersistNetworkd, "eth0"); err != nil || ok || c != "" {
		t.Fatalf("absent = %q,%v,%v", c, ok, err)
	}
	// write (creates the dir) then read back
	if err := p.SetPersisted(ctx, PersistNetworkd, "eth0", "[Match]\nName=eth0\n"); err != nil {
		t.Fatal(err)
	}
	c, ok, err := p.GetPersisted(ctx, PersistNetworkd, "eth0")
	if err != nil || !ok || c != "[Match]\nName=eth0\n" {
		t.Errorf("round-trip = %q,%v,%v", c, ok, err)
	}
	// the file is at the deterministic path
	if _, err := os.Stat(filepath.Join(networkdDir, "10-kscore-eth0.network")); err != nil {
		t.Errorf("expected file at deterministic path: %v", err)
	}
}

func TestLinuxProvider_DetectBackend(t *testing.T) {
	dir := t.TempDir()
	origNP := netplanDir
	defer func() { netplanDir = origNP }()
	p := &linuxProvider{}
	ctx := context.Background()

	// netplan dir absent → networkd
	netplanDir = filepath.Join(dir, "no-netplan")
	if b, err := p.DetectBackend(ctx); err != nil || b != PersistNetworkd {
		t.Errorf("no netplan dir → networkd; got %q,%v", b, err)
	}
	// netplan dir present → netplan
	netplanDir = filepath.Join(dir, "netplan")
	if err := os.MkdirAll(netplanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if b, err := p.DetectBackend(ctx); err != nil || b != PersistNetplan {
		t.Errorf("netplan dir present → netplan; got %q,%v", b, err)
	}
}
