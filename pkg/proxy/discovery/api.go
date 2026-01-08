// Package discovery provides network discovery for proxied devices.
package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DiscoveryAPI provides HTTP handlers for discovery operations.
type DiscoveryAPI struct {
	discovery *Discovery
}

// NewDiscoveryAPI creates a new discovery API.
func NewDiscoveryAPI(discovery *Discovery) *DiscoveryAPI {
	return &DiscoveryAPI{
		discovery: discovery,
	}
}

// RegisterRoutes registers the discovery API routes.
func (api *DiscoveryAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/discovery/scan", api.handleScan)
	mux.HandleFunc("/api/v1/discovery/devices", api.handleDevices)
	mux.HandleFunc("/api/v1/discovery/devices/", api.handleDevice)
	mux.HandleFunc("/api/v1/discovery/pending", api.handlePending)
	mux.HandleFunc("/api/v1/discovery/approved", api.handleApproved)
	mux.HandleFunc("/api/v1/discovery/approve", api.handleApprove)
	mux.HandleFunc("/api/v1/discovery/reject", api.handleReject)
	mux.HandleFunc("/api/v1/discovery/ignore", api.handleIgnore)
	mux.HandleFunc("/api/v1/discovery/profiles", api.handleProfiles)
}

// ScanRequest is the request body for scan operations.
type ScanRequest struct {
	Networks []string `json:"networks,omitempty"`
	Scanners []string `json:"scanners,omitempty"`
}

// handleScan handles POST /api/v1/discovery/scan
func (api *DiscoveryAPI) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	result, err := api.discovery.Scan(ctx)
	if err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, result)
}

// handleDevices handles GET /api/v1/discovery/devices
func (api *DiscoveryAPI) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := api.discovery.GetDiscovered()
	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	})
}

// handleDevice handles GET/DELETE /api/v1/discovery/devices/{id}
func (api *DiscoveryAPI) handleDevice(w http.ResponseWriter, r *http.Request) {
	// Extract device ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/discovery/devices/")
	if path == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		devices := api.discovery.GetDiscovered()
		for _, device := range devices {
			if device.ID == path {
				api.writeJSON(w, http.StatusOK, device)
				return
			}
		}
		http.Error(w, "Device not found", http.StatusNotFound)

	case http.MethodDelete:
		if err := api.discovery.Remove(path); err != nil {
			api.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePending handles GET /api/v1/discovery/pending
func (api *DiscoveryAPI) handlePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := api.discovery.GetPending()
	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	})
}

// handleApproved handles GET /api/v1/discovery/approved
func (api *DiscoveryAPI) handleApproved(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := api.discovery.GetApproved()
	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	})
}

// ApprovalRequest is the request body for approval operations.
type ApprovalRequest struct {
	DeviceID    string `json:"device_id"`
	Profile     string `json:"profile,omitempty"`
	Credentials string `json:"credentials,omitempty"`
}

// handleApprove handles POST /api/v1/discovery/approve
func (api *DiscoveryAPI) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.DeviceID == "" {
		api.writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}

	if err := api.discovery.Approve(req.DeviceID); err != nil {
		api.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "approved",
		"device_id": req.DeviceID,
	})
}

// handleReject handles POST /api/v1/discovery/reject
func (api *DiscoveryAPI) handleReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.DeviceID == "" {
		api.writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}

	if err := api.discovery.Reject(req.DeviceID); err != nil {
		api.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "rejected",
		"device_id": req.DeviceID,
	})
}

// handleIgnore handles POST /api/v1/discovery/ignore
func (api *DiscoveryAPI) handleIgnore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.DeviceID == "" {
		api.writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}

	if err := api.discovery.Ignore(req.DeviceID); err != nil {
		api.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	api.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ignored",
		"device_id": req.DeviceID,
	})
}

// handleProfiles handles GET /api/v1/discovery/profiles
func (api *DiscoveryAPI) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	profiles := DefaultProfiles()
	api.writeJSON(w, http.StatusOK, map[string]interface{}{
		"profiles": profiles,
		"count":    len(profiles),
	})
}

// writeJSON writes a JSON response.
func (api *DiscoveryAPI) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response.
func (api *DiscoveryAPI) writeError(w http.ResponseWriter, status int, message string) {
	api.writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// DiscoveryStats holds discovery statistics.
type DiscoveryStats struct {
	TotalDiscovered int       `json:"total_discovered"`
	PendingCount    int       `json:"pending_count"`
	ApprovedCount   int       `json:"approved_count"`
	RejectedCount   int       `json:"rejected_count"`
	IgnoredCount    int       `json:"ignored_count"`
	LastScanTime    time.Time `json:"last_scan_time,omitempty"`
	ScannerCount    int       `json:"scanner_count"`
	MatcherCount    int       `json:"matcher_count"`
}

// GetStats returns discovery statistics.
func (d *Discovery) GetStats() DiscoveryStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := DiscoveryStats{
		TotalDiscovered: len(d.discovered),
		ApprovedCount:   len(d.approved),
		ScannerCount:    len(d.scanners),
		MatcherCount:    len(d.matchers),
	}

	for _, device := range d.discovered {
		switch device.Status {
		case StatusPending:
			stats.PendingCount++
		case StatusRejected:
			stats.RejectedCount++
		case StatusIgnored:
			stats.IgnoredCount++
		}
	}

	return stats
}

// BulkApprovalRequest is a request to approve multiple devices.
type BulkApprovalRequest struct {
	DeviceIDs []string `json:"device_ids"`
	Profile   string   `json:"profile,omitempty"`
}

// BulkApprovalResult is the result of a bulk approval.
type BulkApprovalResult struct {
	Approved []string          `json:"approved"`
	Failed   map[string]string `json:"failed,omitempty"`
}

// BulkApprove approves multiple devices at once.
func (d *Discovery) BulkApprove(deviceIDs []string) *BulkApprovalResult {
	result := &BulkApprovalResult{
		Approved: make([]string, 0),
		Failed:   make(map[string]string),
	}

	for _, id := range deviceIDs {
		if err := d.Approve(id); err != nil {
			result.Failed[id] = err.Error()
		} else {
			result.Approved = append(result.Approved, id)
		}
	}

	return result
}

// AutoApproveMatching automatically approves all devices matching specified profiles.
func (d *Discovery) AutoApproveMatching(profiles []string) *BulkApprovalResult {
	d.mu.RLock()
	var toApprove []string
	for id, device := range d.discovered {
		if device.Status != StatusPending {
			continue
		}
		for _, profile := range profiles {
			if device.MatchedProfile == profile {
				toApprove = append(toApprove, id)
				break
			}
		}
	}
	d.mu.RUnlock()

	return d.BulkApprove(toApprove)
}

// ExportDiscovered exports discovered devices to JSON.
func (d *Discovery) ExportDiscovered() ([]byte, error) {
	devices := d.GetDiscovered()
	return json.MarshalIndent(map[string]interface{}{
		"devices":    devices,
		"count":      len(devices),
		"exported_at": time.Now(),
	}, "", "  ")
}

// ImportDiscovered imports discovered devices from JSON.
func (d *Discovery) ImportDiscovered(data []byte) error {
	var imported struct {
		Devices []*DiscoveredDevice `json:"devices"`
	}

	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("failed to parse import data: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, device := range imported.Devices {
		// Set import time
		device.LastSeen = time.Now()
		d.discovered[device.ID] = device

		// Add to approved if already approved
		if device.Status == StatusApproved {
			d.approved[device.ID] = device
		}
	}

	return nil
}
