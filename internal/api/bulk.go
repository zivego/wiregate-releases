package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/policy"
	"github.com/zivego/wiregate/pkg/wgadapter"
)

type bulkAgentStateRequest struct {
	Action   string   `json:"action"`
	AgentIDs []string `json:"agent_ids"`
}

type bulkPolicyAssignRequest struct {
	PolicyID string   `json:"policy_id"`
	AgentIDs []string `json:"agent_ids"`
}

type bulkResultItem struct {
	ID      string `json:"id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func (r *Router) handleBulkAgentState(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "agent.bulk_state") {
		return
	}

	var body bulkAgentStateRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	body.Action = strings.TrimSpace(body.Action)
	if body.Action != "disable" && body.Action != "enable" && body.Action != "revoke" {
		writeError(w, http.StatusBadRequest, "validation_failed", "action must be disable, enable, or revoke")
		return
	}
	if len(body.AgentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "agent_ids are required")
		return
	}

	results := make([]bulkResultItem, 0, len(body.AgentIDs))
	for _, rawID := range body.AgentIDs {
		agentID := strings.TrimSpace(rawID)
		if agentID == "" {
			continue
		}
		updated, err := r.enrollmentService.ChangeAgentState(req.Context(), agentID, body.Action)
		if err == nil && updated.Peer != nil && (body.Action == "disable" || body.Action == "revoke") {
			err = r.wgService.ApplyPeer(req.Context(), wgadapter.ApplyPeerInput{
				PeerID:     updated.Peer.ID,
				PublicKey:  updated.Peer.PublicKey,
				AllowedIPs: updated.Peer.AllowedIPs,
				Action:     "remove",
			})
		}
		if err != nil {
			results = append(results, bulkResultItem{ID: agentID, Success: false, Error: err.Error()})
			continue
		}
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "agent." + body.Action,
			ResourceType: "agent",
			ResourceID:   agentID,
			Result:       "success",
			Metadata: map[string]any{
				"bulk": true,
			},
		})
		results = append(results, bulkResultItem{ID: agentID, Success: true})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (r *Router) handleBulkAssignPolicies(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "policy.bulk_assign") {
		return
	}

	var body bulkPolicyAssignRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	body.PolicyID = strings.TrimSpace(body.PolicyID)
	if body.PolicyID == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "policy_id is required")
		return
	}
	if len(body.AgentIDs) == 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "agent_ids are required")
		return
	}

	results := make([]bulkResultItem, 0, len(body.AgentIDs))
	for _, rawID := range body.AgentIDs {
		agentID := strings.TrimSpace(rawID)
		if agentID == "" {
			continue
		}
		_, err := r.policyService.AssignPolicy(req.Context(), policy.AssignPolicyInput{
			AgentID:        agentID,
			AccessPolicyID: body.PolicyID,
		})
		if err != nil {
			if errors.Is(err, policy.ErrAssignmentConflict) {
				results = append(results, bulkResultItem{ID: agentID, Success: true})
				continue
			}
			results = append(results, bulkResultItem{ID: agentID, Success: false, Error: err.Error()})
			continue
		}
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "policy.assign",
			ResourceType: "agent",
			ResourceID:   agentID,
			Result:       "success",
			Metadata: map[string]any{
				"access_policy_id": body.PolicyID,
				"bulk":             true,
			},
		})
		results = append(results, bulkResultItem{ID: agentID, Success: true})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
