package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/enrollment"
	"github.com/zivego/wiregate/internal/policy"
	"github.com/zivego/wiregate/internal/security"
	"github.com/zivego/wiregate/pkg/wgadapter"
)

type peerInventoryResponse struct {
	ID              string   `json:"id"`
	PublicKey       string   `json:"public_key"`
	AssignedAddress string   `json:"assigned_address,omitempty"`
	AllowedIPs      []string `json:"allowed_ips,omitempty"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"created_at"`
}

type agentInventoryResponse struct {
	ID                        string                 `json:"id"`
	Hostname                  string                 `json:"hostname"`
	Platform                  string                 `json:"platform"`
	Status                    string                 `json:"status"`
	IsOnline                  bool                   `json:"is_online"`
	TrafficMode               string                 `json:"traffic_mode"`
	TrafficModeOverride       string                 `json:"traffic_mode_override,omitempty"`
	GatewayMode               string                 `json:"gateway_mode"`
	LastSeenAt                string                 `json:"last_seen_at,omitempty"`
	ReportedVersion           string                 `json:"reported_version,omitempty"`
	ReportedConfigFingerprint string                 `json:"reported_config_fingerprint,omitempty"`
	LastApplyStatus           string                 `json:"last_apply_status,omitempty"`
	LastApplyError            string                 `json:"last_apply_error,omitempty"`
	LastAppliedAt             string                 `json:"last_applied_at,omitempty"`
	CreatedAt                 string                 `json:"created_at"`
	Peer                      *peerInventoryResponse `json:"peer,omitempty"`
}

// agentOnlineThreshold is the maximum time since the last check-in
// for an agent to be considered online. Agents check in every 30s,
// so 2 minutes allows for ~4 missed heartbeats.
const agentOnlineThreshold = 2 * time.Minute

func (r *Router) handleListAgents(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	pageSize := 50
	pageSizeRaw := req.URL.Query().Get("page_size")
	if pageSizeRaw == "" {
		pageSizeRaw = req.URL.Query().Get("limit")
	}
	if pageSizeRaw != "" {
		parsed, err := strconv.Atoi(pageSizeRaw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "validation_failed", "page_size must be a positive integer")
			return
		}
		pageSize = parsed
	}

	page, err := r.enrollmentService.ListAgentsPage(req.Context(), enrollment.ListAgentsFilter{
		Status:   req.URL.Query().Get("status"),
		Platform: req.URL.Query().Get("platform"),
		Query:    req.URL.Query().Get("q"),
		Limit:    pageSize,
		Cursor:   req.URL.Query().Get("cursor"),
	})
	if errors.Is(err, enrollment.ErrInvalidAgentCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid agent cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list agents error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list agents")
		return
	}

	resp := make([]agentInventoryResponse, 0, len(page.Agents))
	for _, agent := range page.Agents {
		item, mapErr := r.agentInventoryToResponse(req.Context(), agent)
		if mapErr != nil {
			r.logger.Printf("list agents map traffic mode error: %v", mapErr)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to list agents")
			return
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": resp, "next_cursor": page.NextCursor})
}

func (r *Router) handleGetAgent(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	agentID := strings.TrimPrefix(req.URL.Path, "/api/v1/agents/")
	if agentID == "" || strings.Contains(agentID, "/") {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}

	agent, err := r.enrollmentService.GetAgent(req.Context(), agentID)
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if err != nil {
		r.logger.Printf("get agent error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load agent")
		return
	}

	resp, err := r.agentInventoryToResponse(req.Context(), agent)
	if err != nil {
		r.logger.Printf("get agent map traffic mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load agent")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type agentCheckInRequest struct {
	Version           string                  `json:"version"`
	LocalState        *agentLocalStateRequest `json:"local_state,omitempty"`
	RotationPublicKey string                  `json:"rotation_public_key,omitempty"`
}

type agentLocalStateRequest struct {
	ReportedConfigFingerprint string `json:"reported_config_fingerprint,omitempty"`
	LastApplyStatus           string `json:"last_apply_status,omitempty"`
	LastApplyError            string `json:"last_apply_error,omitempty"`
	LastAppliedAt             string `json:"last_applied_at,omitempty"`
}

type agentCheckInResponse struct {
	Agent               agentInventoryResponse   `json:"agent"`
	ReconfigureRequired bool                     `json:"reconfigure_required"`
	DesiredState        string                   `json:"desired_state"`
	RotationRequired    bool                     `json:"rotation_required"`
	WireGuardConfig     *wireGuardConfigResponse `json:"wireguard_config,omitempty"`
	Version             string                   `json:"version,omitempty"`
	CheckedInAt         string                   `json:"checked_in_at"`
	Update              *agentUpdateDirective    `json:"update,omitempty"`
}

type agentUpdateDirective struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type patchAgentStateRequest struct {
	Action string `json:"action"`
}

type patchAgentTrafficModeRequest struct {
	Mode string `json:"mode"`
}

type reissueAgentResponse struct {
	ID              string   `json:"id"`
	Model           string   `json:"model"`
	Scope           string   `json:"scope"`
	Status          string   `json:"status"`
	BoundIdentity   string   `json:"bound_identity,omitempty"`
	AccessPolicyIDs []string `json:"access_policy_ids,omitempty"`
	ExpiresAt       string   `json:"expires_at"`
	CreatedByUserID string   `json:"created_by_user_id"`
	CreatedAt       string   `json:"created_at"`
	Token           string   `json:"token"`
}

func (r *Router) handleAgentPost(w http.ResponseWriter, req *http.Request) {
	agentID, action, valid := parseAgentAction(req.URL.Path)
	if !valid {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	switch action {
	case "check-in":
		r.handleAgentCheckIn(w, req, agentID)
	case "gateway-mode":
		r.handlePostAgentGatewayMode(w, req, agentID)
	case "reissue":
		r.handleReissueAgent(w, req, agentID)
	case "rotate":
		r.handleRotateAgent(w, req, agentID)
	default:
		r.handleNotImplemented(w, req)
	}
}

func (r *Router) handlePatchAgent(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}

	agentID, action, valid := parseAgentAction(req.URL.Path)
	if !valid || (action != "state" && action != "traffic-mode") {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	limitKey := "agent.state"
	if action == "traffic-mode" {
		limitKey = "agent.traffic_mode"
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, limitKey) {
		return
	}

	if action == "traffic-mode" {
		var body patchAgentTrafficModeRequest
		if err := r.decodeJSONBody(w, req, &body); err != nil {
			writeDecodeError(w, err)
			return
		}

		trafficMode, err := r.policyService.SetAgentTrafficModeOverride(req.Context(), agentID, body.Mode)
		if errors.Is(err, policy.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "agent not found")
			return
		}
		if errors.Is(err, policy.ErrInvalidPolicyRequest) {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if err != nil {
			r.logger.Printf("set agent traffic mode error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to update traffic mode")
			return
		}

		updated, err := r.enrollmentService.GetAgent(req.Context(), agentID)
		if errors.Is(err, enrollment.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "agent not found")
			return
		}
		if err != nil {
			r.logger.Printf("reload agent after traffic mode update error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load updated agent")
			return
		}

		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "agent.traffic_mode.update",
			ResourceType: "agent",
			ResourceID:   agentID,
			Result:       "success",
			Metadata: map[string]any{
				"effective_mode": trafficMode.Effective,
				"override_mode":  trafficMode.Override,
			},
		})

		resp, err := r.agentInventoryToResponse(req.Context(), updated)
		if err != nil {
			r.logger.Printf("map updated agent traffic mode error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load updated agent")
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	var body patchAgentStateRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if strings.TrimSpace(body.Action) == "revoke" {
		if handled := r.maybeRequestDualApproval(w, req, claims, "agent.revoke", "agent", agentID, map[string]any{"action": "revoke"}); handled {
			return
		}
	}

	updated, err := r.enrollmentService.ChangeAgentState(req.Context(), agentID, strings.TrimSpace(body.Action))
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, enrollment.ErrAgentStateConflict) || errors.Is(err, enrollment.ErrInvalidEnrollmentRequest) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("change agent state error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update agent state")
		return
	}

	if updated.Peer != nil && (body.Action == "disable" || body.Action == "revoke") {
		if err := r.wgService.ApplyPeer(req.Context(), wgadapter.ApplyPeerInput{
			PeerID:     updated.Peer.ID,
			PublicKey:  updated.Peer.PublicKey,
			AllowedIPs: updated.Peer.AllowedIPs,
			Action:     "remove",
		}); err != nil {
			r.logger.Printf("remove runtime peer on %s error: %v", body.Action, err)
			writeError(w, http.StatusInternalServerError, "runtime_apply_failed", "failed to remove runtime peer")
			return
		}
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "agent." + body.Action,
		ResourceType: "agent",
		ResourceID:   updated.ID,
		Result:       "success",
		Metadata: map[string]any{
			"agent_status": updated.Status,
			"peer_status":  peerStatusFromInventory(updated.Peer),
		},
	})
	resp, err := r.agentInventoryToResponse(req.Context(), updated)
	if err != nil {
		r.logger.Printf("change agent state map traffic mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update agent state")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleAgentCheckIn(w http.ResponseWriter, req *http.Request, agentID string) {
	var body agentCheckInRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	token, ok := extractToken(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid agent token")
		return
	}

	result, err := r.enrollmentService.CheckInAgent(req.Context(), enrollment.CheckInAgentInput{
		AgentID:           agentID,
		Token:             token,
		Version:           body.Version,
		LocalState:        agentLocalStateFromRequest(body.LocalState),
		RotationPublicKey: body.RotationPublicKey,
	})
	if errors.Is(err, enrollment.ErrInvalidEnrollmentRequest) {
		r.recordRotationAuditFailure(req.Context(), agentID, body.RotationPublicKey, err)
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if errors.Is(err, enrollment.ErrEnrollmentConflict) {
		r.recordRotationAuditFailure(req.Context(), agentID, body.RotationPublicKey, err)
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, enrollment.ErrInvalidAgentToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid agent token")
		return
	}
	if err != nil {
		r.recordRotationAuditFailure(req.Context(), agentID, body.RotationPublicKey, err)
		r.logger.Printf("agent check-in error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to process check-in")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		Action:       "agent.check_in",
		ResourceType: "agent",
		ResourceID:   result.Agent.ID,
		Result:       "success",
		Metadata: map[string]any{
			"version":              result.Version,
			"reconfigure_required": result.ReconfigureRequired,
			"desired_state":        result.DesiredState,
			"rotation_required":    result.RotationRequired,
		},
	})
	if body.RotationPublicKey != "" && result.Agent.Peer != nil && result.Agent.Peer.PublicKey == body.RotationPublicKey {
		r.recordAuditEvent(req.Context(), audit.Event{
			Action:       "agent.rotate.complete",
			ResourceType: "agent",
			ResourceID:   result.Agent.ID,
			Result:       "success",
			Metadata: map[string]any{
				"peer_id":             result.Agent.Peer.ID,
				"peer_status":         result.Agent.Peer.Status,
				"rotation_public_key": body.RotationPublicKey,
			},
		})
	}

	responseAgent, err := r.agentInventoryToResponse(req.Context(), result.Agent)
	if err != nil {
		r.logger.Printf("agent check-in map traffic mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to process check-in")
		return
	}

	resp := agentCheckInResponse{
		Agent:               responseAgent,
		ReconfigureRequired: result.ReconfigureRequired,
		DesiredState:        result.DesiredState,
		RotationRequired:    result.RotationRequired,
		WireGuardConfig:     wireGuardConfigToResponse(result.WireGuardConfig),
		Version:             result.Version,
		CheckedInAt:         result.CheckedInAt.Format(time.RFC3339),
	}
	if r.agentUpdateVersion != "" && r.agentUpdateBaseURL != "" && body.Version != "" && body.Version != r.agentUpdateVersion {
		platform := result.Agent.Platform
		resp.Update = &agentUpdateDirective{
			Version: r.agentUpdateVersion,
			URL:     strings.TrimRight(r.agentUpdateBaseURL, "/") + "/wiregate-agent-" + platform + "-" + r.agentUpdateVersion,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleGetUpdateManifest(w http.ResponseWriter, req *http.Request) {
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	if r.agentUpdateVersion == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"update_available": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"update_available": true,
		"version":          r.agentUpdateVersion,
		"base_url":         r.agentUpdateBaseURL,
		"platforms":        []string{"linux", "windows"},
	})
}

func (r *Router) handleReissueAgent(w http.ResponseWriter, req *http.Request, agentID string) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "agent.reissue") {
		return
	}
	if handled := r.maybeRequestDualApproval(w, req, claims, "agent.reissue", "agent", agentID, nil); handled {
		return
	}

	token, rawToken, _, err := r.enrollmentService.ReissueAgentEnrollment(req.Context(), agentID, claims.UserID)
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, enrollment.ErrAgentStateConflict) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("reissue agent enrollment error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to reissue enrollment")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "agent.reissue",
		ResourceType: "agent",
		ResourceID:   agentID,
		Result:       "success",
		Metadata: map[string]any{
			"token_id":          token.ID,
			"bound_identity":    token.BoundIdentity,
			"access_policy_ids": token.AccessPolicyIDs,
		},
	})

	writeJSON(w, http.StatusOK, reissueAgentResponse{
		ID:              token.ID,
		Model:           token.Model,
		Scope:           token.Scope,
		Status:          token.Status,
		BoundIdentity:   token.BoundIdentity,
		AccessPolicyIDs: token.AccessPolicyIDs,
		ExpiresAt:       token.ExpiresAt.Format(time.RFC3339),
		CreatedByUserID: token.CreatedByUserID,
		CreatedAt:       token.CreatedAt.Format(time.RFC3339),
		Token:           rawToken,
	})
}

func (r *Router) handleRotateAgent(w http.ResponseWriter, req *http.Request, agentID string) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "agent.rotate.request") {
		return
	}
	if handled := r.maybeRequestDualApproval(w, req, claims, "agent.rotate", "agent", agentID, nil); handled {
		return
	}

	updated, err := r.enrollmentService.MarkAgentRotationPending(req.Context(), agentID)
	if errors.Is(err, enrollment.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	if errors.Is(err, enrollment.ErrAgentStateConflict) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("rotate agent error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to request rotation")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "agent.rotate.request",
		ResourceType: "agent",
		ResourceID:   updated.ID,
		Result:       "success",
		Metadata: map[string]any{
			"peer_id":     peerIDFromInventory(updated.Peer),
			"peer_status": peerStatusFromInventory(updated.Peer),
		},
	})
	resp, err := r.agentInventoryToResponse(req.Context(), updated)
	if err != nil {
		r.logger.Printf("rotate agent map traffic mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to request rotation")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) agentInventoryToResponse(ctx context.Context, agent enrollment.AgentInventory) (agentInventoryResponse, error) {
	resp := agentInventoryResponse{
		ID:                        agent.ID,
		Hostname:                  agent.Hostname,
		Platform:                  agent.Platform,
		Status:                    agent.Status,
		GatewayMode:               agent.GatewayMode,
		ReportedVersion:           agent.ReportedVersion,
		ReportedConfigFingerprint: agent.ReportedConfigFingerprint,
		LastApplyStatus:           agent.LastApplyStatus,
		LastApplyError:            agent.LastApplyError,
		CreatedAt:                 agent.CreatedAt.Format(time.RFC3339),
	}
	if agent.LastSeenAt != nil {
		resp.LastSeenAt = agent.LastSeenAt.Format(time.RFC3339)
		resp.IsOnline = agent.Status != "disabled" && agent.Status != "revoked" && time.Since(*agent.LastSeenAt) < agentOnlineThreshold
	}
	if agent.LastAppliedAt != nil {
		resp.LastAppliedAt = agent.LastAppliedAt.Format(time.RFC3339)
	}
	if agent.Peer != nil {
		resp.Peer = &peerInventoryResponse{
			ID:              agent.Peer.ID,
			PublicKey:       agent.Peer.PublicKey,
			AssignedAddress: agent.Peer.AssignedAddress,
			AllowedIPs:      agent.Peer.AllowedIPs,
			Status:          agent.Peer.Status,
			CreatedAt:       agent.Peer.CreatedAt.Format(time.RFC3339),
		}
	}
	if r.policyService != nil {
		trafficMode, err := r.policyService.GetAgentTrafficMode(ctx, agent.ID)
		if err != nil {
			return agentInventoryResponse{}, err
		}
		resp.TrafficMode = trafficMode.Effective
		resp.TrafficModeOverride = trafficMode.Override
	}
	if resp.TrafficMode == "" {
		resp.TrafficMode = policy.TrafficModeStandard
	}
	if resp.GatewayMode == "" {
		resp.GatewayMode = "disabled"
	}
	return resp, nil
}

func agentLocalStateFromRequest(req *agentLocalStateRequest) enrollment.AgentLocalState {
	if req == nil {
		return enrollment.AgentLocalState{}
	}
	var lastAppliedAt *time.Time
	if req.LastAppliedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, req.LastAppliedAt); err == nil {
			lastAppliedAt = &parsed
		}
	}
	return enrollment.AgentLocalState{
		ReportedConfigFingerprint: req.ReportedConfigFingerprint,
		LastApplyStatus:           req.LastApplyStatus,
		LastApplyError:            req.LastApplyError,
		LastAppliedAt:             lastAppliedAt,
	}
}

func parseAgentAction(path string) (agentID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/agents/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func peerStatusFromInventory(peer *enrollment.PeerInventory) string {
	if peer == nil {
		return ""
	}
	return peer.Status
}

func peerIDFromInventory(peer *enrollment.PeerInventory) string {
	if peer == nil {
		return ""
	}
	return peer.ID
}

func (r *Router) recordRotationAuditFailure(ctx context.Context, agentID, rotationPublicKey string, err error) {
	if strings.TrimSpace(rotationPublicKey) == "" {
		return
	}
	r.recordAuditEvent(ctx, audit.Event{
		Action:       "agent.rotate.failure",
		ResourceType: "agent",
		ResourceID:   agentID,
		Result:       "failure",
		Metadata: map[string]any{
			"rotation_public_key": rotationPublicKey,
			"error":               err.Error(),
		},
	})
}

func (r *Router) maybeRequestDualApproval(w http.ResponseWriter, req *http.Request, claims auth.Claims, approvalAction, resourceType, resourceID string, payload map[string]any) bool {
	if r.securityService == nil {
		return false
	}
	policy, err := r.securityService.GetPolicy(req.Context())
	if err != nil {
		r.logger.Printf("get dual approval policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to evaluate security policy")
		return true
	}
	if !policy.DualApprovalEnabled {
		return false
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required when dual approval is enabled")
		return true
	}
	approval, err := r.securityService.CreateApproval(req.Context(), approvalAction, resourceType, resourceID, claims.UserID, payload)
	if errors.Is(err, security.ErrApprovalConflict) {
		writeError(w, http.StatusConflict, "conflict", "approval already pending for this action")
		return true
	}
	if err != nil {
		r.logger.Printf("create approval error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create approval")
		return true
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "security.approval.request",
		ResourceType: "security_approval",
		ResourceID:   approval.ID,
		Result:       "success",
		Metadata: map[string]any{
			"action":        approval.Action,
			"resource_type": approval.ResourceType,
			"resource_id":   approval.ResourceID,
		},
	})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"approval": securityApprovalToResponse(approval),
	})
	return true
}

type wireGuardConfigResponse struct {
	InterfaceAddress string   `json:"interface_address"`
	ServerEndpoint   string   `json:"server_endpoint"`
	ServerPublicKey  string   `json:"server_public_key"`
	AllowedIPs       []string `json:"allowed_ips"`
	DNSServers       []string `json:"dns_servers,omitempty"`
	DNSSearchDomains []string `json:"dns_search_domains,omitempty"`
}

func wireGuardConfigToResponse(cfg *enrollment.WireGuardConfig) *wireGuardConfigResponse {
	if cfg == nil {
		return nil
	}
	return &wireGuardConfigResponse{
		InterfaceAddress: cfg.InterfaceAddress,
		ServerEndpoint:   cfg.ServerEndpoint,
		ServerPublicKey:  cfg.ServerPublicKey,
		AllowedIPs:       cfg.AllowedIPs,
		DNSServers:       cfg.DNSServers,
		DNSSearchDomains: cfg.DNSSearchDomains,
	}
}
