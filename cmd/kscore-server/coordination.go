// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"go.keystone-core.io/keystone-core/internal/cluster"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/identity"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
	"go.keystone-core.io/keystone-core/pkg/version"
)

// coordinationRuntime is the server↔server CoordinationService stack
// (Epic 13 tasks 12/13): a dedicated mTLS-only gRPC listener plus the
// peer-dialing CoordinationClient. It is the NATS-down recovery
// channel — peers exchange health/leader/recovery over mTLS when the
// event bus is unavailable.
//
// Constructed only when cluster.coordination.listen_addr is set; the
// channel is strictly opt-in and mTLS-only, so it requires the
// identity provider to be enabled.
type coordinationRuntime struct {
	log *slog.Logger

	listener   net.Listener
	grpcServer *grpc.Server
	client     *controlplane.CoordinationClient

	membership *cluster.MembershipManager
	observer   *coordPeerReconciler

	// tlsCancel tears down the server + client TLS rotation watchers.
	tlsCancel func()

	stopOnce sync.Once
}

// startCoordination builds and starts the coordination stack on
// clusterRuntime when an operator opts in via
// cluster.coordination.listen_addr. Empty listen_addr ⇒ no channel
// (returns nil, leaving r.coord nil). The channel is mTLS-only by
// contract, so identityProvider must be non-nil.
func (r *clusterRuntime) startCoordination(ctx context.Context, cfg config.ClusterConfig, identityProvider *identity.EmbeddedProvider, log *slog.Logger) error {
	if cfg.Coordination.ListenAddr == "" {
		return nil
	}
	if identityProvider == nil {
		return errors.New("cluster.coordination.listen_addr is set but identity is disabled; the server↔server channel is mTLS-only and requires identity.enabled=true")
	}

	// Server side: strict mTLS — every peer must present a verified
	// client certificate (CoordinationService rejects non-mTLS callers
	// at the RPC layer too, but the listener enforces it at the
	// transport).
	serverTLS, serverCancel, err := identity.BuildServerTLSConfig(ctx, identityProvider, identity.ServerRoleControlPlane,
		&identity.ServerTLSOptions{
			ClientAuth: tls.RequireAndVerifyClientCert,
			Logger:     log,
		})
	if err != nil {
		return fmt.Errorf("coordination server tls: %w", err)
	}
	// Client side: presents this node's control-plane SVID on dial and
	// verifies peers against the trust bundle (SPIFFE, not DNS).
	clientTLS, clientCancel, err := identity.BuildClientTLSConfig(ctx, identityProvider,
		&identity.ClientTLSOptions{Logger: log})
	if err != nil {
		serverCancel()
		return fmt.Errorf("coordination client tls: %w", err)
	}
	tlsCancel := func() { serverCancel(); clientCancel() }

	lis, err := net.Listen("tcp", cfg.Coordination.ListenAddr)
	if err != nil {
		tlsCancel()
		return fmt.Errorf("coordination listen %q: %w", cfg.Coordination.ListenAddr, err)
	}

	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	coordSrv := &controlplane.CoordinationGRPCServer{
		Leader:      r.election,
		Members:     r.membership,
		Shards:      r.shardStore,
		SelfID:      r.memberID,
		SelfVersion: version.Get().Version,
		// Health + NATS are left nil until the HealthMonitor-backed
		// quorum status lands (a later PR): ClusterHealth then returns
		// Unavailable and NATSStatus reports "unknown", which is the
		// correct best-effort answer on the recovery channel.
	}
	v1.RegisterCoordinationServiceServer(gs, coordSrv)

	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Warn("coordination listener stopped with error", "err", err)
		}
	}()

	client, err := controlplane.NewCoordinationClient(controlplane.CoordinationClientConfig{
		SelfID:            r.memberID,
		DialOptions:       []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(clientTLS))},
		HeartbeatInterval: cfg.Coordination.HeartbeatInterval,
		HeartbeatTimeout:  cfg.Coordination.HeartbeatTimeout,
		FailureThreshold:  cfg.Coordination.FailureThreshold,
		RetryMax:          cfg.Coordination.RetryMax,
		RetryBaseDelay:    cfg.Coordination.RetryBaseDelay,
		RetryMaxDelay:     cfg.Coordination.RetryMaxDelay,
		Logger:            log,
	})
	if err != nil {
		gs.Stop()
		tlsCancel()
		return fmt.Errorf("coordination client: %w", err)
	}
	if err := client.Start(ctx); err != nil {
		gs.Stop()
		tlsCancel()
		return fmt.Errorf("coordination client start: %w", err)
	}

	cr := &coordinationRuntime{
		log:        log,
		listener:   lis,
		grpcServer: gs,
		client:     client,
		membership: r.membership,
		tlsCancel:  tlsCancel,
	}

	// Reconcile the dial pool from membership. Register the observer
	// BEFORE the initial seed so a member that joins between the two
	// is not missed (AddPeer is idempotent for an unchanged addr).
	cr.observer = &coordPeerReconciler{client: client, selfID: r.memberID, log: log}
	r.membership.AddObserver(cr.observer)
	if err := cr.seedPeers(ctx); err != nil {
		// A failed seed is not fatal — the observer will converge the
		// pool as membership events arrive — but surface it.
		log.Warn("coordination: initial peer seed failed; pool will converge via membership events", "err", err)
	}

	r.coord = cr
	log.Info("coordination channel started",
		"listen_addr", cfg.Coordination.ListenAddr, "member_id", r.memberID)
	return nil
}

// seedPeers loads the current membership and points the dial pool at
// every other member that advertises an address.
func (cr *coordinationRuntime) seedPeers(ctx context.Context) error {
	members, err := cr.membership.LoadMembers(ctx)
	if err != nil {
		return err
	}
	want := make(map[string]string, len(members))
	for _, m := range members {
		if m.ID == cr.observer.selfID || m.Addr == "" {
			continue
		}
		want[m.ID] = m.Addr
	}
	return cr.client.SetPeers(want)
}

// stopAccepting stops serving + dialing peers. Used as the
// GracefulShutdown StopAccepting hook (drain start) and is the same
// teardown stop performs — both route through stop's sync.Once so
// either order is safe.
func (cr *coordinationRuntime) stopAccepting(ctx context.Context) error {
	cr.stop(ctx)
	return nil
}

// stop tears the coordination stack down: deregister the membership
// observer, stop the client, gracefully stop the gRPC server (bounded
// by ctx), and cancel the TLS watchers. Idempotent + nil-safe.
func (cr *coordinationRuntime) stop(ctx context.Context) {
	if cr == nil {
		return
	}
	cr.stopOnce.Do(func() {
		if cr.observer != nil {
			cr.membership.RemoveObserver(cr.observer)
		}
		if cr.client != nil {
			if err := cr.client.Stop(ctx); err != nil {
				cr.log.Warn("coordination client stop", "err", err)
			}
		}
		cr.gracefulStop(ctx)
		if cr.tlsCancel != nil {
			cr.tlsCancel()
		}
	})
}

// gracefulStop drains in-flight RPCs via GracefulStop but falls back
// to a hard Stop if ctx expires first, so a stuck peer can't block
// shutdown past the budget.
func (cr *coordinationRuntime) gracefulStop(ctx context.Context) {
	if cr.grpcServer == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		cr.grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		cr.grpcServer.Stop()
		<-done
	}
}

// coordPeerReconciler keeps the CoordinationClient dial pool in sync
// with membership: a joined/updated member with an advertised address
// is dialled; a departed member is dropped. Self is never dialled.
type coordPeerReconciler struct {
	client *controlplane.CoordinationClient
	selfID string
	log    *slog.Logger
}

func (p *coordPeerReconciler) OnMembershipChange(ev cluster.MemberEvent) {
	m := ev.Member
	if m.ID == p.selfID {
		return
	}
	switch ev.Type {
	case cluster.MemberJoined, cluster.MemberUpdated:
		if m.Addr == "" {
			return
		}
		if err := p.client.AddPeer(m.ID, m.Addr); err != nil {
			p.log.Warn("coordination: add peer failed", "member_id", m.ID, "addr", m.Addr, "err", err)
		}
	case cluster.MemberLeft:
		p.client.RemovePeer(m.ID)
	}
}
