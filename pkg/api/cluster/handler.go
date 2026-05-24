// SPDX-License-Identifier: Apache-2.0

// Package cluster exposes REST routes for the cluster domain
// (Epic 13 task 15) — the operator-facing topology + backup
// surface. Watch streams stay gRPC-only (the events-REST
// precedent).
//
// JSON DTOs serialise the member status as the canonical lowercase
// name (the events/policy REST convention). Each backing provider
// may be nil — the routes that need it return HTTP 503. AddMember
// (POST /members) is 501 by contract: members self-register on
// start (no "add" in the etcd membership model). Wiring real
// providers at kscore-server boot is the deferred
// "Cluster gRPC services boot registration" ROADMAP item; until
// then the routes are mounted but 503.
//
// Authentication + RBAC are enforced upstream by the auth
// middleware (Epic 03); this handler trusts requests past them.
package cluster

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

// StatusProvider reports cluster status.
type StatusProvider interface {
	ClusterName() string
	Quorate() bool
	LeaderID() string
	Members() ([]cluster.Member, error)
}

// LeaderProvider exposes leader read + transfer.
type LeaderProvider interface {
	LeaderID() string
	IsLeader() bool
	TransferLeadership() error
}

// MembersProvider exposes member reads + eviction.
type MembersProvider interface {
	List() ([]cluster.Member, error)
	Get(id string) (cluster.Member, error)
	Evict(id string) error
}

// RebalanceProvider triggers a rebalance.
type RebalanceProvider interface {
	Rebalance() (moved int, err error)
}

// BackupProvider creates/restores cluster snapshots.
type BackupProvider interface {
	CreateBackup() (snapshot []byte, err error)
	RestoreBackup(snapshot []byte, force bool) (applied int, err error)
}

// ClusterProviders bundles the (individually nilable) backends.
type ClusterProviders struct {
	Status    StatusProvider
	Leader    LeaderProvider
	Members   MembersProvider
	Rebalance RebalanceProvider
	Backup    BackupProvider
}

// Handler exposes the cluster-domain REST routes.
type Handler struct{ p ClusterProviders }

// NewHandler returns a Handler. Pass a zero ClusterProviders for
// the not-yet-wired case (every route then returns 503).
func NewHandler(p ClusterProviders) *Handler { return &Handler{p: p} }

// Register installs the cluster-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cluster/status", h.status)
	mux.HandleFunc("GET /api/v1/cluster/leader", h.leader)
	mux.HandleFunc("POST /api/v1/cluster/leader/transfer", h.transfer)
	mux.HandleFunc("GET /api/v1/cluster/members", h.listMembers)
	mux.HandleFunc("POST /api/v1/cluster/members", addMemberUnsupported)
	mux.HandleFunc("GET /api/v1/cluster/members/{id}", h.getMember)
	mux.HandleFunc("DELETE /api/v1/cluster/members/{id}", h.removeMember)
	mux.HandleFunc("POST /api/v1/cluster/rebalance", h.rebalance)
	mux.HandleFunc("POST /api/v1/cluster/backup", h.backup)
	mux.HandleFunc("POST /api/v1/cluster/restore", h.restore)
}

type memberDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Status  string `json:"status"`
	Role    string `json:"role"`
}

func toMemberDTO(m cluster.Member, leaderID string) memberDTO {
	role := "follower"
	if m.ID == leaderID && leaderID != "" {
		role = "leader"
	}
	return memberDTO{ID: m.ID, Name: m.Name, Address: m.Addr, Status: string(m.Status), Role: role}
}

func (h *Handler) status(w http.ResponseWriter, _ *http.Request) {
	if h.p.Status == nil {
		unavailable(w)
		return
	}
	members, err := h.p.Status.Members()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	healthy := 0
	for _, m := range members {
		if m.Status == cluster.MemberHealthy {
			healthy++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cluster_id":    h.p.Status.ClusterName(),
		"leader_id":     h.p.Status.LeaderID(),
		"member_count":  len(members),
		"healthy_count": healthy,
		"quorum":        h.p.Status.Quorate(),
	})
}

func (h *Handler) leader(w http.ResponseWriter, _ *http.Request) {
	if h.p.Leader == nil {
		unavailable(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"leader_id": h.p.Leader.LeaderID(),
		"is_self":   h.p.Leader.IsLeader(),
	})
}

func (h *Handler) transfer(w http.ResponseWriter, _ *http.Request) {
	if h.p.Leader == nil {
		unavailable(w)
		return
	}
	if !h.p.Leader.IsLeader() {
		writeErr(w, http.StatusConflict, "not the leader; transfer must run on the leader")
		return
	}
	if err := h.p.Leader.TransferLeadership(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transferred": true})
}

func (h *Handler) listMembers(w http.ResponseWriter, _ *http.Request) {
	if h.p.Members == nil {
		unavailable(w)
		return
	}
	ms, err := h.p.Members.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lid := ""
	if h.p.Status != nil {
		lid = h.p.Status.LeaderID()
	}
	out := make([]memberDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, toMemberDTO(m, lid))
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out, "total_count": len(out)})
}

func (h *Handler) getMember(w http.ResponseWriter, r *http.Request) {
	if h.p.Members == nil {
		unavailable(w)
		return
	}
	m, err := h.p.Members.Get(r.PathValue("id"))
	if errors.Is(err, cluster.ErrMemberNotFound) {
		writeErr(w, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lid := ""
	if h.p.Status != nil {
		lid = h.p.Status.LeaderID()
	}
	writeJSON(w, http.StatusOK, toMemberDTO(m, lid))
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	if h.p.Members == nil {
		unavailable(w)
		return
	}
	if err := h.p.Members.Evict(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func addMemberUnsupported(w http.ResponseWriter, _ *http.Request) {
	writeErr(w, http.StatusNotImplemented,
		"members self-register on start (no AddMember in the etcd membership model)")
}

func (h *Handler) rebalance(w http.ResponseWriter, _ *http.Request) {
	if h.p.Rebalance == nil {
		unavailable(w)
		return
	}
	moved, err := h.p.Rebalance.Rebalance()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reassigned_agents": moved})
}

func (h *Handler) backup(w http.ResponseWriter, _ *http.Request) {
	if h.p.Backup == nil {
		unavailable(w)
		return
	}
	blob, err := h.p.Backup.CreateBackup()
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error()) // e.g. not the leader
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	if h.p.Backup == nil {
		unavailable(w)
		return
	}
	var body struct {
		Snapshot []byte `json:"snapshot"`
		Force    bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	applied, err := h.p.Backup.RestoreBackup(body.Snapshot, body.Force)
	if errors.Is(err, cluster.ErrInvalidSnapshot) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "applied": applied})
}

func unavailable(w http.ResponseWriter) {
	writeErr(w, http.StatusServiceUnavailable, "cluster subsystem not enabled")
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
