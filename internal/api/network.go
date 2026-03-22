package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/enrollment"
	wirenetwork "github.com/zivego/wiregate/internal/network"
)

type postAgentGatewayModeRequest struct {
	Mode string `json:"mode"`
}

type diagnosticsSummaryResponse struct {
	TotalAgents   int `json:"total_agents"`
	DirectAgents  int `json:"direct_agents"`
	RelayAgents   int `json:"relay_agents"`
	GatewayAgents int `json:"gateway_agents"`
	ConflictCount int `json:"conflict_count"`
}

type agentDiagnosticResponse struct {
	AgentID                 string   `json:"agent_id"`
	Hostname                string   `json:"hostname"`
	Platform                string   `json:"platform"`
	AgentStatus             string   `json:"agent_status"`
	PeerID                  string   `json:"peer_id,omitempty"`
	PeerStatus              string   `json:"peer_status,omitempty"`
	TrafficMode             string   `json:"traffic_mode"`
	GatewayMode             string   `json:"gateway_mode"`
	RouteProfile            string   `json:"route_profile"`
	PathMode                string   `json:"path_mode"`
	GatewayAssignmentStatus string   `json:"gateway_assignment_status"`
	DriftState              string   `json:"drift_state,omitempty"`
	AllowedDestinations     []string `json:"allowed_destinations,omitempty"`
	RouteConflicts          []string `json:"route_conflicts,omitempty"`
	LastSeenAt              string   `json:"last_seen_at,omitempty"`
}

type diagnosticsSnapshotResponse struct {
	GeneratedAt    string                     `json:"generated_at"`
	RelayAvailable bool                       `json:"relay_available"`
	RelayStatus    string                     `json:"relay_status"`
	Summary        diagnosticsSummaryResponse `json:"summary"`
	Agents         []agentDiagnosticResponse  `json:"agents"`
}

func (r *Router) handlePostAgentGatewayMode(w http.ResponseWriter, req *http.Request, agentID string) {
	if r.networkService == nil {
		writeError(w, http.StatusNotFound, "not_found", "network gateway controls are unavailable")
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
	if !r.limitSensitiveAction(w, req, claims.UserID, "agent.gateway_mode") {
		return
	}

	var body postAgentGatewayModeRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	mode, err := r.networkService.SetAgentGatewayMode(req.Context(), agentID, body.Mode)
	if errors.Is(err, wirenetwork.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, wirenetwork.ErrInvalidGatewayMode) {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("set agent gateway mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update gateway mode")
		return
	}

	updated, err := r.enrollmentService.GetAgent(req.Context(), agentID)
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if err != nil {
		r.logger.Printf("reload agent after gateway mode update error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load updated agent")
		return
	}

	resp, err := r.agentInventoryToResponse(req.Context(), updated)
	if err != nil {
		r.logger.Printf("map updated agent after gateway mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load updated agent")
		return
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "agent.gateway_mode.update",
		ResourceType: "agent",
		ResourceID:   agentID,
		Result:       "success",
		Metadata: map[string]any{
			"gateway_mode": mode,
		},
	})
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleGetNetworkDiagnostics(w http.ResponseWriter, req *http.Request) {
	if r.networkService == nil {
		writeError(w, http.StatusNotFound, "not_found", "network diagnostics are unavailable")
		return
	}
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	snapshot, err := r.networkService.GetDiagnosticsSnapshot(req.Context())
	if err != nil {
		r.logger.Printf("network diagnostics error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load network diagnostics")
		return
	}
	writeJSON(w, http.StatusOK, diagnosticsSnapshotToResponse(snapshot))
}

func diagnosticsSnapshotToResponse(snapshot wirenetwork.DiagnosticsSnapshot) diagnosticsSnapshotResponse {
	resp := diagnosticsSnapshotResponse{
		GeneratedAt:    snapshot.GeneratedAt.Format(time.RFC3339Nano),
		RelayAvailable: snapshot.RelayAvailable,
		RelayStatus:    snapshot.RelayStatus,
		Summary: diagnosticsSummaryResponse{
			TotalAgents:   snapshot.Summary.TotalAgents,
			DirectAgents:  snapshot.Summary.DirectAgents,
			RelayAgents:   snapshot.Summary.RelayAgents,
			GatewayAgents: snapshot.Summary.GatewayAgents,
			ConflictCount: snapshot.Summary.ConflictCount,
		},
		Agents: make([]agentDiagnosticResponse, 0, len(snapshot.Agents)),
	}
	for _, agent := range snapshot.Agents {
		item := agentDiagnosticResponse{
			AgentID:                 agent.AgentID,
			Hostname:                agent.Hostname,
			Platform:                agent.Platform,
			AgentStatus:             agent.AgentStatus,
			PeerID:                  agent.PeerID,
			PeerStatus:              agent.PeerStatus,
			TrafficMode:             agent.TrafficMode,
			GatewayMode:             agent.GatewayMode,
			RouteProfile:            agent.RouteProfile,
			PathMode:                agent.PathMode,
			GatewayAssignmentStatus: agent.GatewayAssignmentStatus,
			DriftState:              agent.DriftState,
			AllowedDestinations:     agent.AllowedDestinations,
			RouteConflicts:          agent.RouteConflicts,
		}
		if agent.LastSeenAt != nil {
			item.LastSeenAt = agent.LastSeenAt.Format(time.RFC3339)
		}
		resp.Agents = append(resp.Agents, item)
	}
	return resp
}
