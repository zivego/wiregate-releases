package api

import (
	"net/http"
	"time"

	"github.com/zivego/wiregate/internal/capacity"
	"github.com/zivego/wiregate/internal/version"
)

type capacityInventoryResponse struct {
	Users                    int `json:"users"`
	Agents                   int `json:"agents"`
	Peers                    int `json:"peers"`
	AccessPolicies           int `json:"access_policies"`
	PolicyAssignments        int `json:"policy_assignments"`
	EnrollmentTokens         int `json:"enrollment_tokens"`
	PendingSecurityApprovals int `json:"pending_security_approvals"`
	RecentlySeenAgents       int `json:"recently_seen_agents"`
	FailedAgents             int `json:"failed_agents"`
	DriftedPeers             int `json:"drifted_peers"`
}

type capacitySessionResponse struct {
	ActiveSessions     int `json:"active_sessions"`
	IdleTimeoutMinutes int `json:"idle_timeout_minutes"`
}

type capacityAuditResponse struct {
	TotalEvents   int    `json:"total_events"`
	OldestEventAt string `json:"oldest_event_at,omitempty"`
	NewestEventAt string `json:"newest_event_at,omitempty"`
}

type capacityLoggingResponse struct {
	QueueCapacity       int `json:"queue_capacity"`
	CurrentQueued       int `json:"current_queued"`
	SinksTotal          int `json:"sinks_total"`
	EnabledSinks        int `json:"enabled_sinks"`
	DegradedSinks       int `json:"degraded_sinks"`
	DroppedEvents       int `json:"dropped_events"`
	TotalDelivered      int `json:"total_delivered"`
	TotalFailed         int `json:"total_failed"`
	ConsecutiveFailures int `json:"consecutive_failures"`
}

type capacityStorageResponse struct {
	AnalyticsRollups int `json:"analytics_rollups"`
	LogDeadLetters   int `json:"log_dead_letters"`
}

type capacitySnapshotResponse struct {
	GeneratedAt    string                    `json:"generated_at"`
	DatabaseEngine string                    `json:"database_engine"`
	Inventory      capacityInventoryResponse `json:"inventory"`
	Sessions       capacitySessionResponse   `json:"sessions"`
	Audit          capacityAuditResponse     `json:"audit"`
	Logging        capacityLoggingResponse   `json:"logging"`
	Storage        capacityStorageResponse   `json:"storage"`
}

func (r *Router) handleGetSystemCapacity(w http.ResponseWriter, req *http.Request) {
	if r.capacityService == nil {
		writeError(w, http.StatusNotFound, "not_found", "capacity summary is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}

	snapshot, err := r.capacityService.GetSnapshot(req.Context())
	if err != nil {
		r.logger.Printf("system capacity error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load system capacity")
		return
	}
	writeJSON(w, http.StatusOK, capacitySnapshotToResponse(snapshot))
}

func (r *Router) handleGetClusterStatus(w http.ResponseWriter, req *http.Request) {
	if r.cluster == nil {
		writeError(w, http.StatusNotFound, "not_found", "cluster status is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}

	status, err := r.cluster.Status(req.Context())
	if err != nil {
		r.logger.Printf("cluster status error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load cluster status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleGetSystemVersion returns the running server version (unauthenticated).
func (r *Router) handleGetSystemVersion(w http.ResponseWriter, req *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version":    version.Version,
		"commit_sha": version.CommitSHA,
		"build_time": version.BuildTime,
	})
}

// handleCheckForUpdate checks for available updates (admin only).
func (r *Router) handleCheckForUpdate(w http.ResponseWriter, req *http.Request) {
	u := getUpdater()
	if u == nil {
		writeError(w, http.StatusNotFound, "not_found", "server updates are not enabled")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	result, err := u.CheckForUpdate(req.Context())
	if err != nil {
		r.logger.Printf("update check error: %v", err)
		writeError(w, http.StatusBadGateway, "update_check_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleApplyUpdate starts the update process (admin only).
func (r *Router) handleApplyUpdate(w http.ResponseWriter, req *http.Request) {
	u := getUpdater()
	if u == nil {
		writeError(w, http.StatusNotFound, "not_found", "server updates are not enabled")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	var body struct {
		TargetVersion string `json:"target_version"`
	}
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		return
	}
	if body.TargetVersion == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "target_version is required")
		return
	}
	if err := u.StartUpdate(req.Context(), body.TargetVersion); err != nil {
		writeError(w, http.StatusConflict, "update_conflict", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, u.Status())
}

// handleUpdateStatus returns current update progress (admin only).
func (r *Router) handleUpdateStatus(w http.ResponseWriter, req *http.Request) {
	u := getUpdater()
	if u == nil {
		writeError(w, http.StatusNotFound, "not_found", "server updates are not enabled")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	writeJSON(w, http.StatusOK, u.Status())
}

func capacitySnapshotToResponse(snapshot capacity.Snapshot) capacitySnapshotResponse {
	resp := capacitySnapshotResponse{
		GeneratedAt:    snapshot.GeneratedAt.Format(time.RFC3339Nano),
		DatabaseEngine: snapshot.DatabaseEngine,
		Inventory: capacityInventoryResponse{
			Users:                    snapshot.Inventory.Users,
			Agents:                   snapshot.Inventory.Agents,
			Peers:                    snapshot.Inventory.Peers,
			AccessPolicies:           snapshot.Inventory.AccessPolicies,
			PolicyAssignments:        snapshot.Inventory.PolicyAssignments,
			EnrollmentTokens:         snapshot.Inventory.EnrollmentTokens,
			PendingSecurityApprovals: snapshot.Inventory.PendingApprovals,
			RecentlySeenAgents:       snapshot.Inventory.RecentlySeenAgents,
			FailedAgents:             snapshot.Inventory.FailedAgents,
			DriftedPeers:             snapshot.Inventory.DriftedPeers,
		},
		Sessions: capacitySessionResponse{
			ActiveSessions:     snapshot.Sessions.ActiveSessions,
			IdleTimeoutMinutes: snapshot.Sessions.IdleTimeoutMinutes,
		},
		Audit: capacityAuditResponse{
			TotalEvents: snapshot.Audit.TotalEvents,
		},
		Logging: capacityLoggingResponse{
			QueueCapacity:       snapshot.Logging.QueueCapacity,
			CurrentQueued:       snapshot.Logging.CurrentQueued,
			SinksTotal:          snapshot.Logging.SinksTotal,
			EnabledSinks:        snapshot.Logging.EnabledSinks,
			DegradedSinks:       snapshot.Logging.DegradedSinks,
			DroppedEvents:       snapshot.Logging.DroppedEvents,
			TotalDelivered:      snapshot.Logging.TotalDelivered,
			TotalFailed:         snapshot.Logging.TotalFailed,
			ConsecutiveFailures: snapshot.Logging.ConsecutiveFailures,
		},
		Storage: capacityStorageResponse{
			AnalyticsRollups: snapshot.Storage.AnalyticsRollups,
			LogDeadLetters:   snapshot.Storage.LogDeadLetters,
		},
	}
	if snapshot.Audit.OldestEventAt != nil {
		resp.Audit.OldestEventAt = snapshot.Audit.OldestEventAt.Format(time.RFC3339Nano)
	}
	if snapshot.Audit.NewestEventAt != nil {
		resp.Audit.NewestEventAt = snapshot.Audit.NewestEventAt.Format(time.RFC3339Nano)
	}
	return resp
}
