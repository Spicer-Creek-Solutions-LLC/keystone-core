// SPDX-License-Identifier: Apache-2.0

// Package netpersist holds the shared boot-survive ("persist")
// machinery for the network-family stdlib modules (network, route,
// bond, bridge, vlan): the systemd-networkd / netplan file paths, the
// atomic read/write primitives, and the backend auto-detector.
//
// Each module renders its own file *content* (a `.network` /
// `.netdev` body or a netplan document); this package only owns the
// where (paths, base directories) and the how (read, atomic write,
// detect) so that logic is written once rather than duplicated per
// module.
//
// It is deliberately plain (no build tags): the file primitives compile
// and run everywhere, and the network-family modules guard their
// platform-specific runtime ops (iproute2) separately, so a persist call
// is never reached on a non-Linux host.
package netpersist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Backends. The empty string is "runtime-only" (no persistent file).
// Auto resolves to one of the concrete backends via DetectBackend.
const (
	Networkd = "networkd"
	Netplan  = "netplan"
	Auto     = "auto"
)

// ValidBackend reports whether s is a concrete or auto backend.
func ValidBackend(s string) bool {
	switch s {
	case Networkd, Netplan, Auto:
		return true
	}
	return false
}

// Base directories. Variables so tests can point them at a tempdir.
// networkd applies `.network` files in lexical order (the first match
// wins), so a low-numbered prefix is authoritative; netplan merges its
// `*.yaml` in lexical order (the last wins), so a high prefix wins.
var (
	NetworkdDir = "/etc/systemd/network"
	NetplanDir  = "/etc/netplan"
)

const (
	networkdPrefix = "10-kscore-"
	netplanPrefix  = "90-kscore-"
)

// NetworkPath is the systemd-networkd `.network` unit this machinery
// manages for an interface.
func NetworkPath(iface string) string {
	return filepath.Join(NetworkdDir, networkdPrefix+iface+".network")
}

// NetdevPath is the systemd-networkd `.netdev` file for a virtual
// interface (bond / bridge / vlan — netplan has no separate netdev
// concept, so this is networkd-only).
func NetdevPath(iface string) string {
	return filepath.Join(NetworkdDir, networkdPrefix+iface+".netdev")
}

// NetplanPath is the netplan YAML document this machinery manages for
// an interface.
func NetplanPath(iface string) string {
	return filepath.Join(NetplanDir, netplanPrefix+iface+".yaml")
}

// NetworkDropinPath is a systemd-networkd drop-in (`<iface>.network.d/
// <slug>.conf`) that networkd merges into the interface's `.network`.
// Drop-ins are how an independent decl (e.g. one route) extends a
// shared interface unit without the first-match-wins collision that
// separate `.network` files would cause.
func NetworkDropinPath(iface, slug string) string {
	return filepath.Join(NetworkdDir, networkdPrefix+iface+".network.d", slug+".conf")
}

// NetplanRoutePath is a per-route netplan document. It carries a
// route-specific name so it is a distinct file from an interface's
// address document — netplan merges the two by their (different) keys.
func NetplanRoutePath(slug string) string {
	return filepath.Join(NetplanDir, netplanPrefix+"route-"+slug+".yaml")
}

// Read returns a managed file's content. exists is false (content "",
// nil error) when the file is absent.
func Read(path string) (content string, exists bool, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a fixed base dir (settable in tests) + a validated interface name
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), true, nil
}

// Write atomically writes content to path, creating the target
// directory if needed.
func Write(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // /etc/systemd/network and /etc/netplan are world-readable system config dirs
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp := path + ".kscore.tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil { //nolint:gosec // network config files are world-readable
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// Remove deletes a managed file. A missing file is not an error (the
// converged state for an `absent` declaration).
func Remove(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// DetectBackend resolves `persist: auto` to a concrete backend: netplan
// when NetplanDir exists, otherwise networkd.
func DetectBackend() (string, error) {
	if fi, err := os.Stat(NetplanDir); err == nil && fi.IsDir() {
		return Netplan, nil
	}
	return Networkd, nil
}
