package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/policy"
)

type accessPolicyResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Destinations     []string `json:"destinations"`
	TrafficMode      string   `json:"traffic_mode"`
	AssignedAgentIDs []string `json:"assigned_agent_ids"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type createAccessPolicyRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Destinations []string `json:"destinations"`
	TrafficMode  string   `json:"traffic_mode"`
}

type updateAccessPolicyRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Destinations []string `json:"destinations"`
	TrafficMode  string   `json:"traffic_mode"`
}

type assignAccessPolicyRequest struct {
	AgentID string `json:"agent_id"`
}

type simulateAccessPolicyRequest struct {
	AgentID             string   `json:"agent_id,omitempty"`
	PolicyIDs           []string `json:"policy_ids,omitempty"`
	TrafficModeOverride string   `json:"traffic_mode_override,omitempty"`
}

type policySimulationResponse struct {
	AgentID              string   `json:"agent_id,omitempty"`
	PolicyIDs            []string `json:"policy_ids"`
	EffectiveTrafficMode string   `json:"effective_traffic_mode"`
	AllowedIPs           []string `json:"allowed_ips"`
	Destinations         []string `json:"destinations"`
	RouteProfile         string   `json:"route_profile"`
}

func (r *Router) handleCreateAccessPolicy(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "access_policy.create") {
		return
	}

	var body createAccessPolicyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	created, err := r.policyService.CreatePolicy(req.Context(), policy.CreatePolicyInput{
		Name:         body.Name,
		Description:  body.Description,
		Destinations: body.Destinations,
		TrafficMode:  body.TrafficMode,
	})
	if errors.Is(err, policy.ErrInvalidPolicyRequest) {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("create access policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create access policy")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "access_policy.create",
		ResourceType: "access_policy",
		ResourceID:   created.ID,
		Result:       "success",
		Metadata: map[string]any{
			"name":         created.Name,
			"destinations": created.Destinations,
			"traffic_mode": created.TrafficMode,
		},
	})
	writeJSON(w, http.StatusCreated, accessPolicyToResponse(created))
}

func (r *Router) handleListAccessPolicies(w http.ResponseWriter, req *http.Request) {
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

	page, err := r.policyService.ListPoliciesPage(req.Context(), policy.ListPoliciesFilter{
		Limit:  pageSize,
		Cursor: req.URL.Query().Get("cursor"),
	})
	if errors.Is(err, policy.ErrInvalidPolicyCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid policy cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list access policies error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list access policies")
		return
	}

	resp := make([]accessPolicyResponse, 0, len(page.Policies))
	for _, item := range page.Policies {
		resp = append(resp, accessPolicyToResponse(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": resp, "next_cursor": page.NextCursor})
}

func (r *Router) handleUpdateAccessPolicy(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "access_policy.update") {
		return
	}

	policyID, action, valid := parsePolicyAction(req.URL.Path)
	if !valid || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "access policy not found")
		return
	}

	var body updateAccessPolicyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	updated, err := r.policyService.UpdatePolicy(req.Context(), policyID, policy.UpdatePolicyInput{
		Name:         body.Name,
		Description:  body.Description,
		Destinations: body.Destinations,
		TrafficMode:  body.TrafficMode,
	})
	if errors.Is(err, policy.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "access policy not found")
		return
	}
	if errors.Is(err, policy.ErrInvalidPolicyRequest) {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("update access policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update access policy")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "access_policy.update",
		ResourceType: "access_policy",
		ResourceID:   updated.ID,
		Result:       "success",
		Metadata: map[string]any{
			"name":         updated.Name,
			"destinations": updated.Destinations,
			"traffic_mode": updated.TrafficMode,
		},
	})
	writeJSON(w, http.StatusOK, accessPolicyToResponse(updated))
}

func (r *Router) handleAssignAccessPolicy(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}

	policyID, action, valid := parsePolicyAction(req.URL.Path)
	if !valid || (action != "assign" && action != "unassign") {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "access_policy."+action) {
		return
	}

	var body assignAccessPolicyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	if action == "assign" {
		assignment, err := r.policyService.AssignPolicy(req.Context(), policy.AssignPolicyInput{
			AgentID:        body.AgentID,
			AccessPolicyID: policyID,
		})
		if errors.Is(err, policy.ErrPolicyNotFound) || errors.Is(err, policy.ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		if errors.Is(err, policy.ErrAssignmentConflict) {
			writeError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		if err != nil {
			r.logger.Printf("assign access policy error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to assign access policy")
			return
		}

		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "access_policy.assign",
			ResourceType: "access_policy",
			ResourceID:   policyID,
			Result:       "success",
			Metadata: map[string]any{
				"assignment_id": assignment.ID,
				"agent_id":      assignment.AgentID,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"id":               assignment.ID,
			"agent_id":         assignment.AgentID,
			"access_policy_id": assignment.AccessPolicyID,
			"status":           assignment.Status,
			"created_at":       assignment.CreatedAt.Format(time.RFC3339),
		})
		return
	}

	assignment, err := r.policyService.UnassignPolicy(req.Context(), policy.UnassignPolicyInput{
		AgentID:        body.AgentID,
		AccessPolicyID: policyID,
	})
	if errors.Is(err, policy.ErrPolicyNotFound) || errors.Is(err, policy.ErrAgentNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if errors.Is(err, policy.ErrAssignmentNotFound) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("unassign access policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to unassign access policy")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "access_policy.unassign",
		ResourceType: "access_policy",
		ResourceID:   policyID,
		Result:       "success",
		Metadata: map[string]any{
			"assignment_id": assignment.ID,
			"agent_id":      assignment.AgentID,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               assignment.ID,
		"agent_id":         assignment.AgentID,
		"access_policy_id": assignment.AccessPolicyID,
		"status":           assignment.Status,
		"created_at":       assignment.CreatedAt.Format(time.RFC3339),
	})
}

func (r *Router) handleSimulateAccessPolicy(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	var body simulateAccessPolicyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	result, err := r.policyService.SimulatePolicyIntent(req.Context(), policy.SimulatePolicyInput{
		AgentID:             body.AgentID,
		PolicyIDs:           body.PolicyIDs,
		TrafficModeOverride: body.TrafficModeOverride,
	})
	if errors.Is(err, policy.ErrAgentNotFound) || errors.Is(err, policy.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	if errors.Is(err, policy.ErrInvalidPolicyRequest) {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("simulate access policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to simulate access policy")
		return
	}

	writeJSON(w, http.StatusOK, policySimulationToResponse(result))
}

func accessPolicyToResponse(item policy.Policy) accessPolicyResponse {
	assignedAgentIDs := make([]string, len(item.AssignedAgentIDs))
	copy(assignedAgentIDs, item.AssignedAgentIDs)
	if len(assignedAgentIDs) == 0 {
		assignedAgentIDs = []string{}
	}

	return accessPolicyResponse{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		Destinations:     item.Destinations,
		TrafficMode:      item.TrafficMode,
		AssignedAgentIDs: assignedAgentIDs,
		CreatedAt:        item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.Format(time.RFC3339),
	}
}

func policySimulationToResponse(item policy.SimulationResult) policySimulationResponse {
	return policySimulationResponse{
		AgentID:              item.AgentID,
		PolicyIDs:            append([]string{}, item.PolicyIDs...),
		EffectiveTrafficMode: item.EffectiveTrafficMode,
		AllowedIPs:           append([]string{}, item.AllowedIPs...),
		Destinations:         append([]string{}, item.Destinations...),
		RouteProfile:         item.RouteProfile,
	}
}

func parsePolicyAction(path string) (policyID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/access-policies/")
	parts := strings.Split(rest, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", false
		}
		return parts[0], "", true
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

func canManagePolicies(role string) bool {
	return role == "admin" || role == "operator"
}
