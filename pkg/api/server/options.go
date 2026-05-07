package server

import (
	"errors"
	"log/slog"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// Options is the constructor input for New. Required fields: Config,
// Logger, Store, NATSManager, Subjects. Everything else has a default.
type Options struct {
	Config      *config.Config
	Logger      *slog.Logger
	Store       state.Store
	NATSManager NATSManager

	// Subjects is the SubjectBuilder threaded into the
	// CommandDispatcher (Epic 05 task 4). cmd/kscore-server pulls this
	// off internal/nats.Manager via Manager.Subjects(). Tests pass a
	// hand-rolled fake.
	Subjects controlplane.Subjects

	// AuthInterceptor wires the gRPC auth chain when non-nil. v1.0
	// task 4 leaves this optional; task 6 pins it on by default.
	AuthInterceptor *auth.InterceptorConfig

	// Clock is injectable for tests (uptime, status-ticker timing).
	// Defaults to time.Now.
	Clock func() time.Time

	// StatusTickerInterval overrides the 30s default from §4.4 step 21.
	StatusTickerInterval time.Duration
}

// validate returns an error if required fields are missing or
// internally inconsistent. Defaults are applied separately by New.
func (o *Options) validate() error {
	if o.Config == nil {
		return errors.New("server: Options.Config is required")
	}
	if o.Logger == nil {
		return errors.New("server: Options.Logger is required")
	}
	if o.Store == nil {
		return errors.New("server: Options.Store is required")
	}
	if o.NATSManager == nil {
		return errors.New("server: Options.NATSManager is required (use NoopNATSManager{} for tests, internal/nats.Manager in production)")
	}
	if o.Subjects == nil {
		return errors.New("server: Options.Subjects is required (Manager.Subjects() in production; a hand-rolled fake in tests)")
	}
	if o.StatusTickerInterval < 0 {
		return errors.New("server: StatusTickerInterval must be non-negative")
	}
	return nil
}
