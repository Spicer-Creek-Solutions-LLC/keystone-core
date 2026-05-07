// Package server is the kscore-server orchestrator: it wires
// internal/state, the controlplane primitives (ConnectionManager,
// CommandDispatcher, BatchDispatcher), the (stubbed) NATS transport,
// and the gRPC + HTTP listeners using the deterministic 21-step init
// sequence documented in PROJECT-DETAILS §4.4.
//
// Usage from cmd/kscore-server/main.go:
//
//	srv, err := server.New(server.Options{
//	    Config: cfg, Logger: log, Store: store,
//	    NATSManager: server.NoopNATSManager{},
//	})
//	if err != nil { return err }
//	if err := srv.Start(ctx); err != nil { return err }
//	<-ctx.Done()
//	return srv.Stop(stopCtx)
//
// Epic 04 task 4 lays the skeleton; later tasks (5-9) flesh out
// dual-stack listeners, the real middleware chain, real health
// endpoint logic, graceful-shutdown timeouts, and production-warning
// surfacing. Concrete gRPC service registrations land with their
// owning epics (06, 07, 09, …).
package server
