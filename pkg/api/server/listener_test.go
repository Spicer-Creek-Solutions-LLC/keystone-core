package server

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestJoinHostPort(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{"0.0.0.0", 8080, "0.0.0.0:8080"},
		{"127.0.0.1", 0, "127.0.0.1:0"},
		{"::", 8080, "[::]:8080"},
		{"::1", 8080, "[::1]:8080"},
		{"2001:db8::1", 9090, "[2001:db8::1]:9090"},
		{"example.com", 8080, "example.com:8080"},
		{"", 8080, ":8080"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := joinHostPort(tc.host, tc.port)
			if got != tc.want {
				t.Errorf("joinHostPort(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}

func TestEnsureIPv6Brackets(t *testing.T) {
	cases := []struct {
		host, want string
	}{
		{"", ""},
		{"127.0.0.1", "127.0.0.1"},
		{"0.0.0.0", "0.0.0.0"},
		{"example.com", "example.com"},
		{"::", "[::]"},
		{"::1", "[::1]"},
		{"2001:db8::1", "[2001:db8::1]"},
		{"[::]", "[::]"},     // idempotent
		{"[::1]", "[::1]"},   // idempotent
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := ensureIPv6Brackets(tc.host)
			if got != tc.want {
				t.Errorf("ensureIPv6Brackets(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

func TestClassifyHost(t *testing.T) {
	cases := []struct {
		host string
		want family
	}{
		{"", familyDualStack},
		{"0.0.0.0", familyDualStack},
		{"127.0.0.1", familyIPv4},
		{"10.0.0.5", familyIPv4},
		{"::", familyIPv6},
		{"[::]", familyIPv6},
		{"::1", familyIPv6},
		{"2001:db8::1", familyIPv6},
		{"example.com", familyHostname},
		{"localhost", familyHostname},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := classifyHost(tc.host)
			if got != tc.want {
				t.Errorf("classifyHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

// requireIPv6 skips the test if the kernel isn't accepting IPv6 binds
// (CI runners with IPv6 disabled are a real environment).
func requireIPv6(t *testing.T) {
	t.Helper()
	probe, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 not available: %v", err)
	}
	probe.Close()
}

func TestListen_IPv4Only(t *testing.T) {
	lns, err := listen("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(lns)
	if len(lns) != 1 {
		t.Fatalf("got %d listeners, want 1", len(lns))
	}
	addr := lns[0].Addr().String()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("addr = %q, want 127.0.0.1:*", addr)
	}
	mustDial(t, addr)
}

func TestListen_IPv6Only(t *testing.T) {
	requireIPv6(t)
	for _, host := range []string{"::1", "[::]"} {
		t.Run(host, func(t *testing.T) {
			lns, err := listen(host, 0)
			if err != nil {
				t.Fatalf("listen %q: %v", host, err)
			}
			defer closeAll(lns)
			if len(lns) != 1 {
				t.Fatalf("got %d listeners, want 1", len(lns))
			}
			addr := lns[0].Addr().String()
			if !strings.HasPrefix(addr, "[") {
				t.Errorf("addr = %q, want IPv6 form", addr)
			}
		})
	}
}

func TestListen_DualStack(t *testing.T) {
	requireIPv6(t)

	lns, err := listen("0.0.0.0", 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer closeAll(lns)

	if len(lns) != 2 {
		t.Fatalf("got %d listeners, want 2 (dual-stack)", len(lns))
	}

	// Listener 0 is the IPv4 (primary).
	v4Addr := lns[0].Addr().String()
	v6Addr := lns[1].Addr().String()
	if !strings.HasPrefix(v4Addr, "0.0.0.0:") {
		t.Errorf("primary addr = %q, want 0.0.0.0:*", v4Addr)
	}
	if !strings.HasPrefix(v6Addr, "[::]") {
		t.Errorf("secondary addr = %q, want [::]:*", v6Addr)
	}

	// Both listeners accept connections from their respective families.
	// The OS routes by address family, so dialing 127.0.0.1:v4Port hits
	// the tcp4 listener; [::1]:v6Port hits the tcp6 listener.
	v4Port := portOf(t, v4Addr)
	v6Port := portOf(t, v6Addr)
	if v4Port == "" || v4Port == "0" || v6Port == "" || v6Port == "0" {
		t.Fatalf("port=0 on bound listener: v4=%q v6=%q", v4Port, v6Port)
	}

	// Drain accepts in goroutines so dials don't block forever.
	acceptedV4 := make(chan struct{}, 1)
	acceptedV6 := make(chan struct{}, 1)
	go acceptOnce(t, lns[0], acceptedV4)
	go acceptOnce(t, lns[1], acceptedV6)

	mustDial(t, "127.0.0.1:"+v4Port)
	mustDial(t, "[::1]:"+v6Port)

	select {
	case <-acceptedV4:
	case <-time.After(time.Second):
		t.Error("tcp4 listener did not accept v4 dial")
	}
	select {
	case <-acceptedV6:
	case <-time.After(time.Second):
		t.Error("tcp6 listener did not accept v6 dial")
	}
}

func TestListen_Hostname(t *testing.T) {
	// "localhost" resolves on every platform; net.Listen will pick a
	// family. Just verify we get exactly one listener back.
	lns, err := listen("localhost", 0)
	if err != nil {
		t.Fatalf("listen localhost: %v", err)
	}
	defer closeAll(lns)
	if len(lns) != 1 {
		t.Errorf("hostname mode listeners = %d, want 1", len(lns))
	}
}

func TestListen_DualStackPartialFailureClosesV4(t *testing.T) {
	requireIPv6(t)

	// Pre-bind on [::]:port_X so the dual-stack listen()'s IPv6 step fails.
	blocker, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		t.Fatalf("blocker bind: %v", err)
	}
	defer blocker.Close()
	port := portOf(t, blocker.Addr().String())

	lns, err := listen("0.0.0.0", atoi(t, port))
	if err == nil {
		closeAll(lns)
		t.Fatalf("expected error; got %d listeners", len(lns))
	}
	if len(lns) != 0 {
		t.Errorf("partial failure leaked %d listeners", len(lns))
	}

	// Verify the v4 port was released — re-binding it should succeed
	// immediately. If listen() leaked the v4 listener, this would
	// either fail with EADDRINUSE or take SO_REUSEADDR-grace-period.
	retry, err := net.Listen("tcp4", "0.0.0.0:"+port)
	if err != nil {
		t.Fatalf("retry bind on freed v4 port: %v", err)
	}
	retry.Close()
}

func TestListen_BadHostname(t *testing.T) {
	_, err := listen("invalid.host.does.not.resolve.invalid", 0)
	if err == nil {
		t.Error("expected DNS resolution error")
	}
}

// ---- helpers --------------------------------------------------------------

func mustDial(t *testing.T, addr string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	c.Close()
}

func acceptOnce(t *testing.T, ln net.Listener, signal chan struct{}) {
	t.Helper()
	c, err := ln.Accept()
	if err != nil {
		return
	}
	c.Close()
	select {
	case signal <- struct{}{}:
	default:
	}
}

func portOf(t *testing.T, addr string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	return port
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("port not numeric: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}
