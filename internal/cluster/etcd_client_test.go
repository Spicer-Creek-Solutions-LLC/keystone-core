package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// freePort returns a currently-free localhost TCP port. There is an
// inherent bind race between close and reuse, but etcd binds within
// milliseconds of StartEtcd so it is acceptable for tests.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// newEmbedded starts an embedded EtcdClient on free ports with a
// temp data dir, and registers Stop via t.Cleanup. The client's
// bound client URL is returned for external-mode tests.
func newEmbedded(t *testing.T) (*EtcdClient, string) {
	t.Helper()
	clientURL := fmt.Sprintf("http://127.0.0.1:%d", freePort(t))
	peerURL := fmt.Sprintf("http://127.0.0.1:%d", freePort(t))

	c, err := NewEtcdClient(EtcdConfig{
		Mode:       ModeEmbedded,
		Name:       "test-node",
		DataDir:    t.TempDir(),
		ClientURLs: []string{clientURL},
		PeerURLs:   []string{peerURL},
		LeaseTTL:   2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start embedded: %v", err)
	}
	t.Cleanup(func() {
		sctx, scancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer scancel()
		_ = c.Stop(sctx)
	})
	return c, clientURL
}

func TestNewEtcdClient_InvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  EtcdConfig
	}{
		{"unknown mode", EtcdConfig{Mode: "raft"}},
		{"embedded no name", EtcdConfig{Mode: ModeEmbedded, DataDir: "/tmp/x", ClientURLs: []string{"http://127.0.0.1:1"}, PeerURLs: []string{"http://127.0.0.1:2"}}},
		{"embedded no datadir", EtcdConfig{Mode: ModeEmbedded, Name: "n", ClientURLs: []string{"http://127.0.0.1:1"}, PeerURLs: []string{"http://127.0.0.1:2"}}},
		{"embedded no client urls", EtcdConfig{Mode: ModeEmbedded, Name: "n", DataDir: "/tmp/x", PeerURLs: []string{"http://127.0.0.1:2"}}},
		{"embedded bad url", EtcdConfig{Mode: ModeEmbedded, Name: "n", DataDir: "/tmp/x", ClientURLs: []string{"://nope"}, PeerURLs: []string{"http://127.0.0.1:2"}}},
		{"external no endpoints", EtcdConfig{Mode: ModeExternal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEtcdClient(tc.cfg)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestEtcdClient_EmbeddedLifecycleAndPrimitives(t *testing.T) {
	c, _ := newEmbedded(t)
	ctx := context.Background()

	// Double start is rejected.
	if err := c.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start = %v, want ErrAlreadyStarted", err)
	}

	// KV round-trip.
	if err := c.Put(ctx, "/k", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	resp, err := c.Get(ctx, "/k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(resp.Kvs) != 1 || string(resp.Kvs[0].Value) != "v" {
		t.Fatalf("Get got %+v, want value v", resp.Kvs)
	}
	if _, err := c.Delete(ctx, "/k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Lease + keepalive + key bound to lease.
	id, err := c.GrantLease(ctx, 0) // default TTL
	if err != nil {
		t.Fatalf("GrantLease: %v", err)
	}
	if id == 0 {
		t.Fatal("lease id 0")
	}
	if err := c.KeepAlive(id); err != nil {
		t.Fatalf("KeepAlive: %v", err)
	}
	if err := c.Put(ctx, "/leased", "x", clientv3.WithLease(id)); err != nil {
		t.Fatalf("Put leased: %v", err)
	}

	// Watch sees a subsequent put.
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	wch, err := c.Watch(wctx, "/w")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := c.Put(ctx, "/w", "1"); err != nil {
		t.Fatalf("Put watched: %v", err)
	}
	select {
	case ev := <-wch:
		if len(ev.Events) == 0 || string(ev.Events[0].Kv.Value) != "1" {
			t.Fatalf("watch event = %+v", ev.Events)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not fire")
	}

	// Txn passthrough (CAS form used by the future ShardStore).
	txn, err := c.Txn(ctx)
	if err != nil {
		t.Fatalf("Txn: %v", err)
	}
	if _, err := txn.If(clientv3.Compare(clientv3.CreateRevision("/cas"), "=", 0)).
		Then(clientv3.OpPut("/cas", "init")).Commit(); err != nil {
		t.Fatalf("Txn commit: %v", err)
	}

	// Revoke is idempotent.
	if err := c.RevokeLease(ctx, id); err != nil {
		t.Fatalf("RevokeLease: %v", err)
	}
	if err := c.RevokeLease(ctx, id); err != nil {
		t.Fatalf("RevokeLease (idempotent) = %v, want nil", err)
	}
	if err := c.RevokeLease(ctx, clientv3.LeaseID(999999)); err != nil {
		t.Fatalf("RevokeLease unknown = %v, want nil", err)
	}

	// Revoking the kept-alive lease closes its keepalive channel
	// while the worker ctx is still live → failure is counted.
	deadline := time.Now().Add(5 * time.Second)
	for c.KeepAliveFailures() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if c.KeepAliveFailures() == 0 {
		t.Fatal("expected KeepAliveFailures >= 1 after the kept-alive lease was revoked")
	}

	// Stop, then post-stop ops fail and restart is refused.
	sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
	defer scancel()
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := c.Put(ctx, "/after", "v"); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Put after Stop = %v, want ErrNotStarted", err)
	}
	if _, err := c.Client(); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Client after Stop = %v, want ErrNotStarted", err)
	}
	if err := c.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Fatalf("Start after Stop = %v, want ErrStopped", err)
	}
	// Stop is idempotent.
	if err := c.Stop(sctx); err != nil {
		t.Fatalf("second Stop = %v, want nil", err)
	}
}

func TestEtcdClient_StopWithoutStartIsNoop(t *testing.T) {
	c, err := NewEtcdClient(EtcdConfig{Mode: ModeExternal, Endpoints: []string{"http://127.0.0.1:1"}})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Fatalf("Stop without Start = %v, want nil", err)
	}
	if err := c.Start(context.Background()); !errors.Is(err, ErrStopped) {
		t.Fatalf("Start after no-op Stop = %v, want ErrStopped", err)
	}
}

func TestEtcdClient_ExternalMode(t *testing.T) {
	// Stand up an embedded node, then connect to its real client
	// listener in external mode.
	_, clientURL := newEmbedded(t)

	ext, err := NewEtcdClient(EtcdConfig{
		Mode:         ModeExternal,
		Endpoints:    []string{clientURL},
		DialTimeout:  3 * time.Second,
		StartTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewEtcdClient external: %v", err)
	}
	ctx := context.Background()
	if err := ext.Start(ctx); err != nil {
		t.Fatalf("Start external: %v", err)
	}
	defer func() {
		sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = ext.Stop(sctx)
	}()

	if err := ext.Put(ctx, "/ext", "ok"); err != nil {
		t.Fatalf("external Put: %v", err)
	}
	got, err := ext.Get(ctx, "/ext")
	if err != nil {
		t.Fatalf("external Get: %v", err)
	}
	if len(got.Kvs) != 1 || string(got.Kvs[0].Value) != "ok" {
		t.Fatalf("external Get = %+v", got.Kvs)
	}
}

func TestEtcdClient_ExternalUnreachable(t *testing.T) {
	ext, err := NewEtcdClient(EtcdConfig{
		Mode:         ModeExternal,
		Endpoints:    []string{fmt.Sprintf("http://127.0.0.1:%d", freePort(t))},
		DialTimeout:  500 * time.Millisecond,
		StartTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	err = ext.Start(context.Background())
	if !errors.Is(err, ErrEtcdUnavailable) {
		t.Fatalf("Start unreachable = %v, want ErrEtcdUnavailable", err)
	}
}

func TestEtcdClient_OpsBeforeStart(t *testing.T) {
	c, err := NewEtcdClient(EtcdConfig{Mode: ModeExternal, Endpoints: []string{"http://127.0.0.1:1"}})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	ctx := context.Background()
	if _, err := c.GrantLease(ctx, time.Second); !errors.Is(err, ErrNotStarted) {
		t.Errorf("GrantLease before Start = %v, want ErrNotStarted", err)
	}
	if err := c.KeepAlive(1); !errors.Is(err, ErrNotStarted) {
		t.Errorf("KeepAlive before Start = %v, want ErrNotStarted", err)
	}
	if _, err := c.Get(ctx, "/x"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Get before Start = %v, want ErrNotStarted", err)
	}
	if _, err := c.Watch(ctx, "/x"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Watch before Start = %v, want ErrNotStarted", err)
	}
	if _, err := c.Txn(ctx); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Txn before Start = %v, want ErrNotStarted", err)
	}
}
