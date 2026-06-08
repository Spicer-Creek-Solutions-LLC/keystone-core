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

func TestNetdevRenderHelpers(t *testing.T) {
	t.Parallel()
	if got := RenderNetdev("bond0", "bond", "[Bond]\nMode=balance-rr\n"); got != ManagedHeader+"[NetDev]\nName=bond0\nKind=bond\n[Bond]\nMode=balance-rr\n" {
		t.Errorf("RenderNetdev = %q", got)
	}
	if got := RenderEnslave("Bond", "bond0"); got != ManagedHeader+"[Network]\nBond=bond0\n" {
		t.Errorf("RenderEnslave = %q", got)
	}
	if got := MinimalBase("eth0"); got != ManagedHeader+"[Match]\nName=eth0\n" {
		t.Errorf("MinimalBase = %q", got)
	}
}

func TestNetplanDevicePath(t *testing.T) {
	origNP := NetplanDir
	NetplanDir = "/np"
	defer func() { NetplanDir = origNP }()
	if got := NetplanDevicePath("bond", "bond0"); got != "/np/90-kscore-bond-bond0.yaml" {
		t.Errorf("NetplanDevicePath = %q", got)
	}
}

func TestRemoveMatching(t *testing.T) {
	dir := t.TempDir()
	origND := NetworkdDir
	NetworkdDir = dir
	defer func() { NetworkdDir = origND }()

	for _, m := range []string{"eth0", "eth1"} {
		if err := Write(NetworkDropinPath(m, "kscore-bond-bond0"), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if err := Write(NetworkDropinPath("eth2", "kscore-bond-other"), "x"); err != nil {
		t.Fatal(err)
	}
	pat := filepath.Join(NetworkdDir, "10-kscore-*.network.d", "kscore-bond-bond0.conf")
	if err := RemoveMatching(pat); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(NetworkDropinPath("eth0", "kscore-bond-bond0")); ok {
		t.Error("matched drop-in should be gone")
	}
	if _, ok, _ := Read(NetworkDropinPath("eth2", "kscore-bond-other")); !ok {
		t.Error("non-matching drop-in should remain")
	}
	// a pattern matching nothing is not an error
	if err := RemoveMatching(filepath.Join(dir, "nomatch-*")); err != nil {
		t.Errorf("no-match RemoveMatching = %v", err)
	}
}

func TestNetdevPersist_Networkd(t *testing.T) {
	dir := t.TempDir()
	origND := NetworkdDir
	NetworkdDir = filepath.Join(dir, "nd")
	defer func() { NetworkdDir = origND }()

	d := NetdevPersist{
		Backend:    Networkd,
		Kind:       "bond",
		Name:       "bond0",
		NetdevBody: RenderNetdev("bond0", "bond", "[Bond]\nMode=active-backup\n"),
		Enslave: []Enslave{
			{Iface: "eth0", Body: RenderEnslave("Bond", "bond0")},
			{Iface: "eth1", Body: RenderEnslave("Bond", "bond0")},
		},
	}
	if drift, err := d.PresentDrift(); err != nil || !drift {
		t.Fatalf("PresentDrift before write = %v,%v; want true", drift, err)
	}
	if err := d.Write(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(NetdevPath("bond0")); !ok {
		t.Error("netdev missing")
	}
	for _, m := range []string{"eth0", "eth1"} {
		if _, ok, _ := Read(NetworkDropinPath(m, "kscore-bond-bond0")); !ok {
			t.Errorf("drop-in for %s missing", m)
		}
		if _, ok, _ := Read(NetworkPath(m)); !ok {
			t.Errorf("base for %s missing", m)
		}
	}
	if drift, err := d.PresentDrift(); err != nil || drift {
		t.Errorf("PresentDrift after write = %v,%v; want false (idempotent)", drift, err)
	}

	// absent struct carries no member list — drift + remove work by glob.
	ad := NetdevPersist{Backend: Networkd, Kind: "bond", Name: "bond0"}
	if drift, err := ad.AbsentDrift(); err != nil || !drift {
		t.Errorf("AbsentDrift = %v,%v; want true", drift, err)
	}
	if err := ad.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(NetdevPath("bond0")); ok {
		t.Error("netdev should be removed")
	}
	for _, m := range []string{"eth0", "eth1"} {
		if _, ok, _ := Read(NetworkDropinPath(m, "kscore-bond-bond0")); ok {
			t.Errorf("drop-in for %s should be removed", m)
		}
		if _, ok, _ := Read(NetworkPath(m)); !ok {
			t.Errorf("base for %s should be left alone", m)
		}
	}
	if drift, err := ad.AbsentDrift(); err != nil || drift {
		t.Errorf("AbsentDrift after remove = %v,%v; want false", drift, err)
	}
}

func TestNetdevPersist_BaseNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	origND := NetworkdDir
	NetworkdDir = filepath.Join(dir, "nd")
	defer func() { NetworkdDir = origND }()

	fuller := "# net module\n[Match]\nName=eth0\n[Network]\nDHCP=yes\n"
	if err := Write(NetworkPath("eth0"), fuller); err != nil {
		t.Fatal(err)
	}
	d := NetdevPersist{
		Backend: Networkd, Kind: "bridge", Name: "br0",
		NetdevBody: RenderNetdev("br0", "bridge", "[Bridge]\nSTP=yes\n"),
		Enslave:    []Enslave{{Iface: "eth0", Body: RenderEnslave("Bridge", "br0")}},
	}
	if err := d.Write(); err != nil {
		t.Fatal(err)
	}
	if base, _, _ := Read(NetworkPath("eth0")); base != fuller {
		t.Errorf("create-if-absent overwrote a present base: %q", base)
	}
}

func TestNetdevPersist_Netplan(t *testing.T) {
	dir := t.TempDir()
	origNP := NetplanDir
	NetplanDir = filepath.Join(dir, "np")
	defer func() { NetplanDir = origNP }()

	body := ManagedHeader + "network:\n  version: 2\n  vlans:\n    vlan100:\n      id: 100\n      link: eth0\n"
	d := NetdevPersist{Backend: Netplan, Kind: "vlan", Name: "vlan100", NetplanBody: body}
	if drift, _ := d.PresentDrift(); !drift {
		t.Error("want drift before write")
	}
	if err := d.Write(); err != nil {
		t.Fatal(err)
	}
	if c, ok, _ := Read(NetplanDevicePath("vlan", "vlan100")); !ok || c != body {
		t.Errorf("netplan body wrong: %q", c)
	}
	if drift, _ := d.PresentDrift(); drift {
		t.Error("idempotent: no drift after write")
	}
	if drift, _ := d.AbsentDrift(); !drift {
		t.Error("AbsentDrift should be true")
	}
	if err := d.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(NetplanDevicePath("vlan", "vlan100")); ok {
		t.Error("netplan file should be removed")
	}
}

func TestNetdevPersist_UnsupportedBackend(t *testing.T) {
	t.Parallel()
	d := NetdevPersist{Backend: "nm", Kind: "bond", Name: "b0"}
	if _, err := d.PresentDrift(); err == nil {
		t.Error("PresentDrift should error")
	}
	if err := d.Write(); err == nil {
		t.Error("Write should error")
	}
	if _, err := d.AbsentDrift(); err == nil {
		t.Error("AbsentDrift should error")
	}
	if err := d.Remove(); err == nil {
		t.Error("Remove should error")
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
