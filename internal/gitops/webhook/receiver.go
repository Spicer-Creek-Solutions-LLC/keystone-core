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
// [Authenticator], dispatches to the matching [Handler], and returns
// 202 on a successful parse.
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
	provider, err := r.reg.Detect(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h, ok := r.reg.Lookup(provider)
	if !ok {
		http.Error(w, "no handler for provider "+provider.String(), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, r.cfg.MaxBodyBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
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
		http.Error(w, "parse: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	r.logger.Info("gitops webhook accepted",
		slog.String("provider", ev.Provider.String()),
		slog.String("application", ev.Application),
		slog.String("status", ev.Status))
	w.WriteHeader(http.StatusAccepted)
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
