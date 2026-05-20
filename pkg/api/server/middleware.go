package server

import (
	"fmt"
	"net/http"
)

// buildHTTPHandler assembles the routing tree for the HTTP server:
//
//	CORS (outermost — sees ALL traffic)
//	  ↓
//	router (path-prefix dispatch)
//	  ├── /health/*  → publicMux (no auth, no rate-limit)
//	  └── otherwise  → auth.HTTPMiddleware → apiMux
//
// CORS lives outside the auth chain so OPTIONS preflight returns 204
// before rate-limit / auth runs (PROJECT-DETAILS §4.4 acceptance
// criterion). Health endpoints are routed off a separate mux so they
// never enter the auth chain at all — operators / load balancers
// don't need credentials to probe liveness.
func (s *Server) buildHTTPHandler() (http.Handler, error) {
	publicMux := http.NewServeMux()
	s.registerHealthEndpoints(publicMux)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/status", s.handleAPIStatus)
	s.registerDomainHandlers(apiMux)

	var apiHandler http.Handler = apiMux
	if s.authInterceptor != nil {
		mw, err := s.authInterceptor.HTTPMiddleware()
		if err != nil {
			return nil, fmt.Errorf("server: build auth HTTP middleware: %w", err)
		}
		apiHandler = mw(apiMux)
	}

	router := http.NewServeMux()
	router.Handle("/health/", publicMux)
	router.Handle("/", apiHandler)

	var handler http.Handler = router
	if s.metrics != nil {
		// Metrics middleware sits outside auth so health probes count
		// too, but inside CORS so OPTIONS preflight isn't recorded as
		// a domain request.
		handler = s.metrics.HTTPMiddleware(nil)(handler)
	}
	if s.cfg.Server.CORS.Enabled {
		return corsMiddleware(s.cfg.Server.CORS)(handler), nil
	}
	return handler, nil
}
