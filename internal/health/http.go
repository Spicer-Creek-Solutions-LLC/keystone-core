package health

import (
	"encoding/json"
	"net/http"
)

// Handler provides HTTP handlers for health check endpoints
type Handler struct {
	manager *Manager
}

// NewHandler creates a new health check HTTP handler
func NewHandler(manager *Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// LivenessHandler returns the liveness probe handler
// Returns 200 OK if the process is running
func (h *Handler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := h.manager.Liveness()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// ReadinessHandler returns the readiness probe handler
// Returns 200 OK if ready to serve traffic, 503 otherwise
func (h *Handler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := h.manager.Readiness()

		w.Header().Set("Content-Type", "application/json")

		// Return 503 if not ready
		if response.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// StatusHandler returns the detailed status handler
// Returns comprehensive system status information
func (h *Handler) StatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := h.manager.Status()

		w.Header().Set("Content-Type", "application/json")

		// Return 503 if unhealthy
		if response.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if response.Status == StatusDegraded {
			w.WriteHeader(http.StatusOK) // Still serving but with warnings
		} else {
			w.WriteHeader(http.StatusOK)
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// RegisterRoutes registers all health check routes on a mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health/live", h.LivenessHandler())
	mux.HandleFunc("/health/ready", h.ReadinessHandler())
	mux.HandleFunc("/health/status", h.StatusHandler())
}

// RegisterRoutesWithPrefix registers health check routes with a custom prefix
func (h *Handler) RegisterRoutesWithPrefix(mux *http.ServeMux, prefix string) {
	if prefix == "" {
		prefix = "/health"
	}

	mux.HandleFunc(prefix+"/live", h.LivenessHandler())
	mux.HandleFunc(prefix+"/ready", h.ReadinessHandler())
	mux.HandleFunc(prefix+"/status", h.StatusHandler())
}
