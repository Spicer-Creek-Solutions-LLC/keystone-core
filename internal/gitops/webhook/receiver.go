package webhook

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// ReceiverConfig is the receiver's runtime configuration. The boot
// path derives it from config.GitOpsWebhookConfig; tests construct it
// directly. Zero MaxBodyBytes falls back to [DefaultMaxBodyBytes].
type ReceiverConfig struct {
	// Addr is the listen address, e.g. ":8081".
	Addr string
	// Path is the single POST route, e.g. "/webhooks".
	Path string
	// MaxBodyBytes caps the request body; larger requests get 413.
	MaxBodyBytes int64
	// Authenticators is the per-source authenticator, keyed by
	// provider. A provider absent from the map (or a nil map) is
	// authenticated with [NoneAuthenticator] — operators are warned
	// in production (config.ProductionWarnings). Build it with
	// [BuildAuthenticators].
	Authenticators map[Provider]Authenticator
	// Emitter re-emits each accepted webhook on the Keystone event
	// bus as `gitops.<provider>.<subtype>`. Nil disables emission
	// (the receiver still authenticates, parses, and 202s) — the
	// dark-until-boot default until the publisher is wired at
	// kscore-server boot.
	Emitter EventEmitter
	// EventSource is the emitted event's Source field (e.g. the
	// server node name). Defaults to "gitops-webhook" when empty.
	EventSource string
}

// DefaultMaxBodyBytes bounds an inbound webhook body (1 MiB). GitHub
// caps payloads at 25 MiB but GitOps deployment events are small;
// 1 MiB is generous and bounds memory per request.
const DefaultMaxBodyBytes = 1 << 20

// shutdownTimeout bounds the graceful drain in [Receiver.Stop] when
// the caller's context has no deadline.
const shutdownTimeout = 5 * time.Second

// Receiver is the GitOps inbound webhook HTTP server. It owns its own
// *http.Server (separate from the main REST API on :8080), auto-detects
// the source provider from request headers via [Registry.Detect],
// reads the body once size-capped, authenticates it with the source's
// [Authenticator], dispatches to the matching [Handler], re-emits the
// normalized event on the Keystone bus (when an [EventEmitter] is
// configured), and returns 202 on a successful parse.
type Receiver struct {
	cfg    ReceiverConfig
	reg    *Registry
	logger *slog.Logger

	srv *http.Server
	ln  net.Listener

	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error
}

// New constructs a Receiver. The listener is opened in [Receiver.Start],
// not here, so construction never fails on a bound port.
func New(cfg ReceiverConfig, reg *Registry, logger *slog.Logger) *Receiver {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if cfg.Path == "" {
		cfg.Path = "/webhooks"
	}
	if cfg.EventSource == "" {
		cfg.EventSource = "gitops-webhook"
	}
	if logger == nil {
		logger = slog.Default()
	}
	r := &Receiver{cfg: cfg, reg: reg, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+cfg.Path, r.handle)
	r.srv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return r
}

// Addr returns the actual listen address. Valid only after a
// successful [Receiver.Start]; useful for tests that bind ":0".
func (r *Receiver) Addr() string {
	if r.ln == nil {
		return r.cfg.Addr
	}
	return r.ln.Addr().String()
}

// Start binds the listener and serves in a background goroutine. It is
// idempotent: repeated calls return the first call's result.
func (r *Receiver) Start(_ context.Context) error {
	r.startOnce.Do(func() {
		ln, err := net.Listen("tcp", r.cfg.Addr)
		if err != nil {
			r.startErr = err
			return
		}
		r.ln = ln
		go func() {
			if serveErr := r.srv.Serve(ln); serveErr != nil &&
				!errors.Is(serveErr, http.ErrServerClosed) {
				r.logger.Error("gitops webhook receiver serve failed",
					slog.String("error", serveErr.Error()))
			}
		}()
		r.logger.Info("gitops webhook receiver listening",
			slog.String("addr", r.Addr()),
			slog.String("path", r.cfg.Path))
	})
	return r.startErr
}

// Stop gracefully drains in-flight requests. Idempotent; safe to call
// without a prior successful Start. Honors ctx's deadline, falling
// back to [shutdownTimeout].
func (r *Receiver) Stop(ctx context.Context) error {
	var err error
	r.stopOnce.Do(func() {
		if r.srv == nil {
			return
		}
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()
		}
		err = r.srv.Shutdown(ctx)
	})
	return err
}

// handle is the single POST route. It auto-detects the provider from
// request headers, enforces the body cap, dispatches to the handler,
// and maps the outcome to an HTTP status. Event-bus emission lands in
// task 4; for now a parsed event is acknowledged with 202.
func (r *Receiver) handle(w http.ResponseWriter, req *http.Request) {
	// Phase B5 finding H1: do not leak internal error strings to
	// unauthenticated callers. The response body carries a generic
	// message; the original error is logged server-side at WARN so
	// operators retain diagnostic visibility.
	provider, err := r.reg.Detect(req)
	if err != nil {
		r.logger.Warn("gitops webhook provider-detect failed",
			slog.String("error", err.Error()))
		http.Error(w, "provider detection failed", http.StatusBadRequest)
		return
	}
	h, ok := r.reg.Lookup(provider)
	if !ok {
		r.logger.Warn("gitops webhook no handler for provider",
			slog.String("provider", provider.String()))
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, r.cfg.MaxBodyBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.logger.Warn("gitops webhook body read failed",
			slog.String("provider", provider.String()),
			slog.String("error", err.Error()))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	auth := r.authFor(provider)
	if authErr := auth.Authenticate(req, body); authErr != nil {
		r.logger.Warn("gitops webhook authentication failed",
			slog.String("provider", provider.String()),
			slog.String("method", string(auth.Method())))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ev, err := h.Parse(req, body)
	if err != nil {
		r.logger.Warn("gitops webhook parse failed",
			slog.String("provider", provider.String()),
			slog.String("error", err.Error()))
		http.Error(w, "failed to process webhook", http.StatusUnprocessableEntity)
		return
	}

	r.logger.Info("gitops webhook accepted",
		slog.String("provider", ev.Provider.String()),
		slog.String("application", ev.Application),
		slog.String("status", ev.Status))
	r.emit(req.Context(), ev)
	w.WriteHeader(http.StatusAccepted)
}

// emit re-publishes an accepted webhook on the event bus. Best-effort
// and synchronous (the runbook-observer precedent): a nil emitter or a
// publish error is logged but does not change the 202 — a sender must
// not be made to retry over an internal bus hiccup once its webhook is
// authenticated and parsed.
func (r *Receiver) emit(ctx context.Context, ev Event) {
	if r.cfg.Emitter == nil {
		return
	}
	kev, err := ToKscoreEvent(ev, r.cfg.EventSource)
	if err != nil {
		r.logger.Warn("gitops webhook event build failed",
			slog.String("provider", ev.Provider.String()),
			slog.String("error", err.Error()))
		return
	}
	if pubErr := r.cfg.Emitter.Publish(ctx, kev); pubErr != nil {
		r.logger.Warn("gitops webhook event publish failed",
			slog.String("type", kev.Type.String()),
			slog.String("error", pubErr.Error()))
	}
}

// authFor returns the configured authenticator for provider, or
// [NoneAuthenticator] when none is configured (default-open; operators
// are warned in production via config.ProductionWarnings).
func (r *Receiver) authFor(p Provider) Authenticator {
	if a, ok := r.cfg.Authenticators[p]; ok && a != nil {
		return a
	}
	return NoneAuthenticator{}
}
