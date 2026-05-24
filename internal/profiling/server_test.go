// SPDX-License-Identifier: Apache-2.0

package profiling

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// listenEphemeral grabs a free localhost port and immediately closes
// the listener so the profiling Server can re-bind. Avoids races on
// fixed-port choices that can collide with other tests.
func listenEphemeral(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	host := addr.IP.String()
	port := addr.Port
	_ = ln.Close()
	return host, port
}

func newEnabledServer(t *testing.T) (*Server, string) {
	t.Helper()
	host, port := listenEphemeral(t)
	srv, err := New(config.ProfilingConfig{
		Enabled:         true,
		Host:            host,
		Port:            port,
		ShutdownTimeout: 2 * time.Second,
	}, discardLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv, "http://" + net.JoinHostPort(host, intToStr(port))
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

func TestNew_Disabled_ReturnsNil(t *testing.T) {
	s, err := New(config.ProfilingConfig{Enabled: false}, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if s != nil {
		t.Fatalf("Server = %v, want nil", s)
	}
}

func TestNew_InvalidConfig_Errors(t *testing.T) {
	_, err := New(config.ProfilingConfig{Enabled: true, Host: "", Port: 6060}, nil)
	if err == nil {
		t.Fatalf("want error for empty host")
	}
}

func TestNilServer_Safe(t *testing.T) {
	var s *Server
	if err := s.Start(context.Background()); err != nil {
		t.Errorf("nil Start = %v, want nil", err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("nil Stop = %v, want nil", err)
	}
	if a := s.Addr(); a != "" {
		t.Errorf("nil Addr = %q, want empty", a)
	}
}

func TestServer_IndexReturns200(t *testing.T) {
	_, base := newEnabledServer(t)
	resp, err := http.Get(base + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"goroutine", "heap", "allocs"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("index body missing %q", want)
		}
	}
}

func TestServer_HeapProfile(t *testing.T) {
	_, base := newEnabledServer(t)
	// debug=1 yields a text dump quickly without holding a CPU profile.
	resp, err := http.Get(base + "/debug/pprof/heap?debug=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Errorf("empty heap profile body")
	}
}

func TestServer_GoroutineProfile(t *testing.T) {
	_, base := newEnabledServer(t)
	resp, err := http.Get(base + "/debug/pprof/goroutine?debug=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestServer_DoubleStart_Rejected(t *testing.T) {
	srv, _ := newEnabledServer(t)
	if err := srv.Start(context.Background()); err == nil {
		t.Errorf("second Start = nil, want error")
	}
}

func TestServer_StopReleasesListener(t *testing.T) {
	host, port := listenEphemeral(t)
	srv, err := New(config.ProfilingConfig{
		Enabled: true, Host: host, Port: port,
		ShutdownTimeout: time.Second,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	addr := "http://" + net.JoinHostPort(host, intToStr(port))

	// Healthy before Stop.
	resp, err := http.Get(addr + "/debug/pprof/")
	if err != nil {
		t.Fatalf("pre-Stop GET: %v", err)
	}
	_ = resp.Body.Close()

	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Post-Stop the server refuses connections. (Re-binding the port
	// would race with TIME_WAIT on the OS socket — verifying the
	// server is gone via a failed request is the stable signal.)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	if resp, err := client.Get(addr + "/debug/pprof/"); err == nil {
		_ = resp.Body.Close()
		t.Errorf("post-Stop GET succeeded; want connection refused")
	}
}

func TestServer_Stop_BeforeStart_NoOp(t *testing.T) {
	host, port := listenEphemeral(t)
	srv, err := New(config.ProfilingConfig{
		Enabled: true, Host: host, Port: port,
		ShutdownTimeout: time.Second,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start = %v, want nil", err)
	}
}

func TestServer_Addr_AfterStart(t *testing.T) {
	srv, _ := newEnabledServer(t)
	if addr := srv.Addr(); addr == "" {
		t.Error("Addr is empty after Start")
	}
}

func TestServer_RuntimeKnobs_AppliedAndRestored(t *testing.T) {
	// Capture baseline.
	prevMutex := runtime.SetMutexProfileFraction(0)
	t.Cleanup(func() { runtime.SetMutexProfileFraction(prevMutex) })

	host, port := listenEphemeral(t)
	srv, err := New(config.ProfilingConfig{
		Enabled:              true,
		Host:                 host,
		Port:                 port,
		MutexProfileFraction: 5,
		BlockProfileRate:     1000,
		ShutdownTimeout:      time.Second,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// SetMutexProfileFraction(-1) is a "read without modifying" idiom;
	// but the public stdlib API doesn't expose a pure read, so we
	// instead Set(N).Set(N) and assert the return is N.
	got := runtime.SetMutexProfileFraction(5)
	if got != 5 {
		t.Errorf("mutex fraction after Start = %d, want 5 (Start applied it)", got)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	// After Stop, the value should be restored to prevMutex.
	got = runtime.SetMutexProfileFraction(prevMutex)
	if got != prevMutex {
		t.Errorf("mutex fraction after Stop = %d, want %d", got, prevMutex)
	}
}

func TestServer_BindFailure_Surfaces(t *testing.T) {
	// Hold the port so the profiling server's Start fails to bind.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	addr := ln.Addr().(*net.TCPAddr)

	srv, err := New(config.ProfilingConfig{
		Enabled: true, Host: addr.IP.String(), Port: addr.Port,
		ShutdownTimeout: time.Second,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	err = srv.Start(context.Background())
	if err == nil {
		t.Fatalf("Start = nil, want bind error")
	}
	// The OS-specific error from net is wrapped by our package as
	// "profiling: listen ...: ...". The substring is the stable
	// signal that we surfaced a bind failure rather than silently
	// swallowing it.
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("err = %v, want wrapped listen error", err)
	}
}
