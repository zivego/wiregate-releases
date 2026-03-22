package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/security"
)

type securityPolicyResponse struct {
	RequiredAdminAMR    string `json:"required_admin_amr,omitempty"`
	RequiredAdminACR    string `json:"required_admin_acr,omitempty"`
	DualApprovalEnabled bool   `json:"dual_approval_enabled"`
}

type patchSecurityPolicyRequest struct {
	RequiredAdminAMR    string `json:"required_admin_amr"`
	RequiredAdminACR    string `json:"required_admin_acr"`
	DualApprovalEnabled bool   `json:"dual_approval_enabled"`
}

type securityApprovalResponse struct {
	ID                string         `json:"id"`
	Action            string         `json:"action"`
	ResourceType      string         `json:"resource_type"`
	ResourceID        string         `json:"resource_id"`
	RequestPayload    map[string]any `json:"request_payload,omitempty"`
	RequestedByUserID string         `json:"requested_by_user_id"`
	ApprovedByUserID  string         `json:"approved_by_user_id,omitempty"`
	RejectedByUserID  string         `json:"rejected_by_user_id,omitempty"`
	Status            string         `json:"status"`
	CreatedAt         string         `json:"created_at"`
	DecidedAt         string         `json:"decided_at,omitempty"`
}

func (r *Router) handleGetSecurityPolicy(w http.ResponseWriter, req *http.Request) {
	if r.securityService == nil {
		writeError(w, http.StatusNotFound, "not_found", "security policy is unavailable")
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

	policy, err := r.securityService.GetPolicy(req.Context())
	if err != nil {
		r.logger.Printf("get security policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load security policy")
		return
	}
	writeJSON(w, http.StatusOK, securityPolicyResponse{
		RequiredAdminAMR:    policy.RequiredAdminAMR,
		RequiredAdminACR:    policy.RequiredAdminACR,
		DualApprovalEnabled: policy.DualApprovalEnabled,
	})
}

func (r *Router) handlePatchSecurityPolicy(w http.ResponseWriter, req *http.Request) {
	if r.securityService == nil {
		writeError(w, http.StatusNotFound, "not_found", "security policy is unavailable")
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
	if !r.limitSensitiveAction(w, req, claims.UserID, "security.policy.update") {
		return
	}

	var body patchSecurityPolicyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	updated, err := r.securityService.UpdatePolicy(req.Context(), security.Policy{
		RequiredAdminAMR:    strings.TrimSpace(body.RequiredAdminAMR),
		RequiredAdminACR:    strings.TrimSpace(body.RequiredAdminACR),
		DualApprovalEnabled: body.DualApprovalEnabled,
	})
	if err != nil {
		r.logger.Printf("update security policy error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update security policy")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "security.policy.update",
		ResourceType: "security_policy",
		ResourceID:   "default",
		Result:       "success",
		Metadata: map[string]any{
			"required_admin_amr":    updated.RequiredAdminAMR,
			"required_admin_acr":    updated.RequiredAdminACR,
			"dual_approval_enabled": updated.DualApprovalEnabled,
		},
	})
	writeJSON(w, http.StatusOK, securityPolicyResponse{
		RequiredAdminAMR:    updated.RequiredAdminAMR,
		RequiredAdminACR:    updated.RequiredAdminACR,
		DualApprovalEnabled: updated.DualApprovalEnabled,
	})
}

func (r *Router) handleListSecurityApprovals(w http.ResponseWriter, req *http.Request) {
	if r.securityService == nil {
		writeError(w, http.StatusNotFound, "not_found", "security approvals are unavailable")
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

	limit := 100
	pageSizeRaw := strings.TrimSpace(req.URL.Query().Get("page_size"))
	if pageSizeRaw == "" {
		pageSizeRaw = strings.TrimSpace(req.URL.Query().Get("limit"))
	}
	if pageSizeRaw != "" {
		parsed, err := strconv.Atoi(pageSizeRaw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "validation_failed", "page_size must be a positive integer")
			return
		}
		limit = parsed
	}
	page, err := r.securityService.ListApprovalsPage(req.Context(), security.ListApprovalsFilter{
		Status: strings.TrimSpace(req.URL.Query().Get("status")),
		Limit:  limit,
		Cursor: strings.TrimSpace(req.URL.Query().Get("cursor")),
	})
	if errors.Is(err, security.ErrInvalidApprovalCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid approval cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list security approvals error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list approvals")
		return
	}

	resp := make([]securityApprovalResponse, 0, len(page.Approvals))
	for _, approval := range page.Approvals {
		resp = append(resp, securityApprovalToResponse(approval))
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": resp, "next_cursor": page.NextCursor})
}

func (r *Router) handleSecurityApprovalPost(w http.ResponseWriter, req *http.Request) {
	if r.securityService == nil {
		writeError(w, http.StatusNotFound, "not_found", "security approvals are unavailable")
		return
	}
	approvalID, action, valid := parseApprovalAction(req.URL.Path)
	if !valid || (action != "approve" && action != "reject") {
		writeError(w, http.StatusNotFound, "not_found", "approval not found")
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
	if !r.limitSensitiveAction(w, req, claims.UserID, "security.approval."+action) {
		return
	}

	if action == "reject" {
		approval, err := r.securityService.MarkRejected(req.Context(), approvalID, claims.UserID)
		if errors.Is(err, security.ErrApprovalNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "approval not found")
			return
		}
		if errors.Is(err, security.ErrApprovalStateConflict) || errors.Is(err, security.ErrApprovalSelfDecision) {
			writeError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		if err != nil {
			r.logger.Printf("reject approval error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to reject approval")
			return
		}
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "security.approval.reject",
			ResourceType: "security_approval",
			ResourceID:   approval.ID,
			Result:       "success",
			Metadata: map[string]any{
				"action":        approval.Action,
				"resource_type": approval.ResourceType,
				"resource_id":   approval.ResourceID,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{"approval": securityApprovalToResponse(approval)})
		return
	}

	result, approval, err := r.executeApprovedSecurityAction(req.Context(), approvalID, claims.UserID)
	if errors.Is(err, security.ErrApprovalNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "approval not found")
		return
	}
	if errors.Is(err, security.ErrApprovalStateConflict) || errors.Is(err, security.ErrApprovalSelfDecision) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("approve approval error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to approve action")
		return
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "security.approval.approve",
		ResourceType: "security_approval",
		ResourceID:   approval.ID,
		Result:       "success",
		Metadata: map[string]any{
			"action":        approval.Action,
			"resource_type": approval.ResourceType,
			"resource_id":   approval.ResourceID,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"approval": securityApprovalToResponse(approval),
		"result":   result,
	})
}

func securityApprovalToResponse(approval security.Approval) securityApprovalResponse {
	resp := securityApprovalResponse{
		ID:                approval.ID,
		Action:            approval.Action,
		ResourceType:      approval.ResourceType,
		ResourceID:        approval.ResourceID,
		RequestPayload:    approval.RequestPayload,
		RequestedByUserID: approval.RequestedByUserID,
		ApprovedByUserID:  approval.ApprovedByUserID,
		RejectedByUserID:  approval.RejectedByUserID,
		Status:            approval.Status,
		CreatedAt:         approval.CreatedAt.Format(time.RFC3339),
	}
	if approval.DecidedAt != nil {
		resp.DecidedAt = approval.DecidedAt.Format(time.RFC3339)
	}
	return resp
}

func parseApprovalAction(path string) (approvalID, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/security/approvals/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}
