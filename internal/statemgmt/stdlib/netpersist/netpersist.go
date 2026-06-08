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

// ManagedHeader prefixes every file the shared render helpers produce.
const ManagedHeader = "# Managed by keystone-core. Do not edit.\n"

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

// NetplanDevicePath is a per-virtual-device netplan document, named by
// kind so a bond/bridge/vlan file is distinct from an interface's
// address document.
func NetplanDevicePath(kind, name string) string {
	return filepath.Join(NetplanDir, netplanPrefix+kind+"-"+name+".yaml")
}

// MinimalBase renders a `[Match]`-only systemd-networkd `.network` base
// unit, written create-if-absent so a drop-in has a unit to merge into.
func MinimalBase(iface string) string {
	return ManagedHeader + "[Match]\nName=" + iface + "\n"
}

// RenderNetdev renders a systemd-networkd `.netdev` unit for a virtual
// device: the `[NetDev]` header (Name + Kind) plus an optional,
// already-formatted kind section (e.g. "[Bond]\nMode=…\n").
func RenderNetdev(name, kind, section string) string {
	return ManagedHeader + "[NetDev]\nName=" + name + "\nKind=" + kind + "\n" + section
}

// RenderEnslave renders a `.network` drop-in that attaches an interface
// to a master device: a single `[Network]` key (Bond= / Bridge= / VLAN=).
func RenderEnslave(key, master string) string {
	return ManagedHeader + "[Network]\n" + key + "=" + master + "\n"
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

// RemoveMatching removes every file matching a glob pattern. A pattern
// that matches nothing is not an error. It is used to clean up the
// per-member enslave drop-ins of a deleted virtual device, whose member
// list the `absent` declaration does not carry.
func RemoveMatching(pattern string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob %s: %w", pattern, err)
	}
	for _, m := range matches {
		if err := Remove(m); err != nil {
			return err
		}
	}
	return nil
}

// fileDrift reports whether path is missing or differs from want.
func fileDrift(path, want string) (bool, error) {
	current, exists, err := Read(path)
	if err != nil {
		return false, err
	}
	return !exists || current != want, nil
}

// Enslave is one member/parent attachment of a virtual device: the
// interface that receives a `[Network]` drop-in pointing at the master.
type Enslave struct {
	Iface string
	Body  string // rendered drop-in content (see RenderEnslave)
}

// NetdevPersist is the persistent footprint of one virtual device
// (bond / bridge / vlan). It centralises the networkd (`.netdev` +
// per-member enslave drop-ins + create-if-absent member bases) and
// netplan (one document) drift/write/remove logic so the netdev-creator
// modules don't each re-implement it.
//
// Kind and Name are always set: the absent path globs the enslave
// drop-ins by them, since an `absent` declaration carries no member
// list. NetdevBody / Enslave / NetplanBody are set for the present path.
type NetdevPersist struct {
	Backend     string // resolved: Networkd | Netplan
	Kind        string // bond | bridge | vlan
	Name        string // device name
	NetdevBody  string // rendered .netdev (networkd)
	Enslave     []Enslave
	NetplanBody string // rendered netplan document
}

func (d NetdevPersist) dropinSlug() string { return "kscore-" + d.Kind + "-" + d.Name }

func (d NetdevPersist) dropinGlob() string {
	return filepath.Join(NetworkdDir, networkdPrefix+"*.network.d", d.dropinSlug()+".conf")
}

// PresentDrift reports whether any managed file for a present device is
// missing or stale. For networkd that is the `.netdev`, each enslave
// drop-in, and each enslaved interface's base unit.
func (d NetdevPersist) PresentDrift() (bool, error) {
	switch d.Backend {
	case Networkd:
		if drift, err := fileDrift(NetdevPath(d.Name), d.NetdevBody); err != nil || drift {
			return drift, err
		}
		for _, e := range d.Enslave {
			if drift, err := fileDrift(NetworkDropinPath(e.Iface, d.dropinSlug()), e.Body); err != nil || drift {
				return drift, err
			}
			if _, baseExists, err := Read(NetworkPath(e.Iface)); err != nil {
				return false, err
			} else if !baseExists {
				return true, nil
			}
		}
		return false, nil
	case Netplan:
		return fileDrift(NetplanDevicePath(d.Kind, d.Name), d.NetplanBody)
	}
	return false, fmt.Errorf("unsupported persist backend %q", d.Backend)
}

// Write renders the present device to disk: the `.netdev`, then for each
// enslaved interface a create-if-absent base unit and the enslave
// drop-in (networkd); or the single netplan document.
func (d NetdevPersist) Write() error {
	switch d.Backend {
	case Networkd:
		if err := Write(NetdevPath(d.Name), d.NetdevBody); err != nil {
			return err
		}
		for _, e := range d.Enslave {
			base := NetworkPath(e.Iface)
			if _, exists, err := Read(base); err != nil {
				return err
			} else if !exists {
				if err := Write(base, MinimalBase(e.Iface)); err != nil {
					return err
				}
			}
			if err := Write(NetworkDropinPath(e.Iface, d.dropinSlug()), e.Body); err != nil {
				return err
			}
		}
		return nil
	case Netplan:
		return Write(NetplanDevicePath(d.Kind, d.Name), d.NetplanBody)
	}
	return fmt.Errorf("unsupported persist backend %q", d.Backend)
}

// AbsentDrift reports whether any managed file for the device still
// exists (and should be removed). It relies on Kind+Name (not the
// member list), so it works for an `absent` declaration.
func (d NetdevPersist) AbsentDrift() (bool, error) {
	switch d.Backend {
	case Networkd:
		if _, exists, err := Read(NetdevPath(d.Name)); err != nil {
			return false, err
		} else if exists {
			return true, nil
		}
		matches, err := filepath.Glob(d.dropinGlob())
		if err != nil {
			return false, fmt.Errorf("glob %s: %w", d.dropinGlob(), err)
		}
		return len(matches) > 0, nil
	case Netplan:
		_, exists, err := Read(NetplanDevicePath(d.Kind, d.Name))
		return exists, err
	}
	return false, fmt.Errorf("unsupported persist backend %q", d.Backend)
}

// Remove deletes the device's managed files: the `.netdev` and every
// enslave drop-in (found by glob, since the member list is absent), or
// the netplan document. Enslaved interfaces' base units are left alone —
// other config may depend on them.
func (d NetdevPersist) Remove() error {
	switch d.Backend {
	case Networkd:
		if err := Remove(NetdevPath(d.Name)); err != nil {
			return err
		}
		return RemoveMatching(d.dropinGlob())
	case Netplan:
		return Remove(NetplanDevicePath(d.Kind, d.Name))
	}
	return fmt.Errorf("unsupported persist backend %q", d.Backend)
}

// DetectBackend resolves `persist: auto` to a concrete backend: netplan
// when NetplanDir exists, otherwise networkd.
func DetectBackend() (string, error) {
	if fi, err := os.Stat(NetplanDir); err == nil && fi.IsDir() {
		return Netplan, nil
	}
	return Networkd, nil
}
