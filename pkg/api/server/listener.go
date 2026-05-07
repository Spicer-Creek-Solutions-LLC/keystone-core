package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// family classifies the listener mode implied by the configured Host.
//
//	familyHostname  — hostname; defer to net.Listen single-stack
//	familyIPv4      — IPv4 literal (loopback, specific NIC, etc.)
//	familyIPv6      — IPv6 literal (including "::" bind-all)
//	familyDualStack — wildcard 0.0.0.0 (or empty); bind v4 + v6
type family int

const (
	familyHostname family = iota
	familyIPv4
	familyIPv6
	familyDualStack
)

// joinHostPort canonicalizes (host, port) into a host:port string.
// Wraps net.JoinHostPort so all kscore-server call sites bracket
// IPv6 literals consistently — joinHostPort("::", 8080) → "[::]:8080".
func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// ensureIPv6Brackets wraps a bare IPv6 literal in [] if not already
// bracketed. IPv4 literals, hostnames, and already-bracketed IPv6
// strings pass through unchanged. Idempotent.
//
// Used in places that accept a pre-formatted host (URL building,
// log lines) rather than splitting + rejoining via joinHostPort.
func ensureIPv6Brackets(host string) string {
	if host == "" {
		return host
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return host
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		// Not an IP literal, or it's IPv4 → no bracketing.
		return host
	}
	return "[" + host + "]"
}

// classifyHost decides the listener family from the configured host.
// Empty string is treated as the dual-stack wildcard for ergonomics
// (matches Go's net.Listen behavior of treating "" as bind-all).
func classifyHost(host string) family {
	switch host {
	case "", "0.0.0.0":
		return familyDualStack
	case "::", "[::]":
		return familyIPv6
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		if ip.To4() != nil {
			return familyIPv4
		}
		return familyIPv6
	}
	return familyHostname
}

// listen binds 1-2 listeners for (host, port) per classifyHost:
//
//	dual-stack — two listeners (tcp4 0.0.0.0:port and tcp6 [::]:port)
//	IPv4-only  — one tcp4 listener
//	IPv6-only  — one tcp6 listener (with bracketing on join)
//	hostname   — one net.Listen("tcp", host:port) — DNS picks family
//
// The first listener returned is the "primary" — its address is the
// one reported via Server.Addrs.GRPC / .HTTP for callers that don't
// inspect the AllGRPC / AllHTTP slices. On partial failure, every
// listener that bound is closed before the error is returned, so
// callers can retry without leaking sockets.
func listen(host string, port int) ([]net.Listener, error) {
	switch classifyHost(host) {
	case familyDualStack:
		return listenDualStack(port)
	case familyIPv4:
		return listenSingle("tcp4", joinHostPort(host, port))
	case familyIPv6:
		// classifyHost normalizes "[::]" → familyIPv6; strip the
		// brackets here so net.Listen sees the canonical form.
		return listenSingle("tcp6", joinHostPort(strings.Trim(host, "[]"), port))
	default:
		return listenSingle("tcp", joinHostPort(host, port))
	}
}

func listenSingle(network, addr string) ([]net.Listener, error) {
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s %s: %w", network, addr, err)
	}
	return []net.Listener{ln}, nil
}

func listenDualStack(port int) ([]net.Listener, error) {
	v4, err := net.Listen("tcp4", joinHostPort("0.0.0.0", port))
	if err != nil {
		return nil, fmt.Errorf("listen tcp4 0.0.0.0:%d: %w", port, err)
	}
	// IPv6 bind on a port-collision-free OS port: when port=0, the v4
	// listener already chose a port; the v6 listener picks its own
	// independently. Tests with port=0 therefore see two distinct
	// ports — that's expected and surfaced via Addrs.AllGRPC/AllHTTP.
	v6, err := net.Listen("tcp6", joinHostPort("::", port))
	if err != nil {
		_ = v4.Close()
		return nil, fmt.Errorf("listen tcp6 [::]:%d: %w", port, err)
	}
	return []net.Listener{v4, v6}, nil
}

// closeAll closes every listener in s, returning the first error
// encountered while still attempting subsequent closes.
func closeAll(s []net.Listener) error {
	var firstErr error
	for _, ln := range s {
		if err := ln.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// addrs returns the bound addresses of every listener in s.
func addrs(s []net.Listener) []string {
	out := make([]string, 0, len(s))
	for _, ln := range s {
		out = append(out, ln.Addr().String())
	}
	return out
}

