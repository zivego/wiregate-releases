package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/enrollment"
)

type createEnrollmentTokenRequest struct {
	Model           string   `json:"model"`
	Scope           string   `json:"scope"`
	BoundIdentity   string   `json:"bound_identity"`
	AccessPolicyIDs []string `json:"access_policy_ids"`
	TTLMinutes      int      `json:"ttl_minutes"`
}

type enrollmentTokenResponse struct {
	ID              string         `json:"id"`
	Model           string         `json:"model"`
	Scope           string         `json:"scope"`
	Status          string         `json:"status"`
	BoundIdentity   string         `json:"bound_identity,omitempty"`
	AccessPolicyIDs []string       `json:"access_policy_ids,omitempty"`
	ExpiresAt       string         `json:"expires_at"`
	UsedAt          string         `json:"used_at,omitempty"`
	RevokedAt       string         `json:"revoked_at,omitempty"`
	CreatedByUserID string         `json:"created_by_user_id"`
	CreatedAt       string         `json:"created_at"`
	Token           string         `json:"token,omitempty"`
	RiskWarning     string         `json:"risk_warning,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

func (r *Router) handleCreateEnrollmentToken(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageEnrollmentTokens(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "enrollment_token.create") {
		return
	}

	var body createEnrollmentTokenRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	var ttl time.Duration
	if body.TTLMinutes > 0 {
		ttl = time.Duration(body.TTLMinutes) * time.Minute
	}

	token, rawToken, warning, err := r.enrollmentService.CreateToken(req.Context(), enrollment.CreateTokenInput{
		Model:           body.Model,
		Scope:           body.Scope,
		BoundIdentity:   body.BoundIdentity,
		AccessPolicyIDs: body.AccessPolicyIDs,
		TTL:             ttl,
		CreatedByUserID: claims.UserID,
	})
	if errors.Is(err, enrollment.ErrInvalidTokenRequest) {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("create enrollment token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create enrollment token")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "enrollment_token.create",
		ResourceType: "enrollment_token",
		ResourceID:   token.ID,
		Result:       "success",
		Metadata: map[string]any{
			"model":             token.Model,
			"scope":             token.Scope,
			"bound_identity":    token.BoundIdentity,
			"access_policy_ids": token.AccessPolicyIDs,
			"expires_at":        token.ExpiresAt.Format(time.RFC3339),
		},
	})

	resp := enrollmentTokenToResponse(token)
	resp.Token = rawToken
	resp.RiskWarning = warning
	writeJSON(w, http.StatusCreated, resp)
}

func (r *Router) handleListEnrollmentTokens(w http.ResponseWriter, req *http.Request) {
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

	page, err := r.enrollmentService.ListTokensPage(req.Context(), enrollment.ListTokensFilter{
		Limit:  pageSize,
		Cursor: req.URL.Query().Get("cursor"),
	})
	if errors.Is(err, enrollment.ErrInvalidTokenCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid enrollment token cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list enrollment tokens error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list enrollment tokens")
		return
	}

	resp := make([]enrollmentTokenResponse, 0, len(page.Tokens))
	for _, token := range page.Tokens {
		resp = append(resp, enrollmentTokenToResponse(token))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": resp, "next_cursor": page.NextCursor})
}

func (r *Router) handleRevokeEnrollmentToken(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageEnrollmentTokens(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "enrollment_token.revoke") {
		return
	}

	tokenID, action, valid := parseEnrollmentTokenAction(req.URL.Path)
	if !valid || action != "revoke" {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	token, err := r.enrollmentService.RevokeToken(req.Context(), tokenID)
	if errors.Is(err, enrollment.ErrTokenNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "enrollment token not found")
		return
	}
	if errors.Is(err, enrollment.ErrTokenStateConflict) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("revoke enrollment token error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke enrollment token")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "enrollment_token.revoke",
		ResourceType: "enrollment_token",
		ResourceID:   token.ID,
		Result:       "success",
		Metadata: map[string]any{
			"model":             token.Model,
			"scope":             token.Scope,
			"bound_identity":    token.BoundIdentity,
			"access_policy_ids": token.AccessPolicyIDs,
		},
	})

	writeJSON(w, http.StatusOK, enrollmentTokenToResponse(token))
}

func enrollmentTokenToResponse(token enrollment.Token) enrollmentTokenResponse {
	resp := enrollmentTokenResponse{
		ID:              token.ID,
		Model:           token.Model,
		Scope:           token.Scope,
		Status:          token.Status,
		BoundIdentity:   token.BoundIdentity,
		AccessPolicyIDs: token.AccessPolicyIDs,
		ExpiresAt:       token.ExpiresAt.Format(time.RFC3339),
		CreatedByUserID: token.CreatedByUserID,
		CreatedAt:       token.CreatedAt.Format(time.RFC3339),
	}
	if token.UsedAt != nil {
		resp.UsedAt = token.UsedAt.Format(time.RFC3339)
	}
	if token.RevokedAt != nil {
		resp.RevokedAt = token.RevokedAt.Format(time.RFC3339)
	}
	return resp
}

func parseEnrollmentTokenAction(path string) (tokenID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/enrollment-tokens/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func canManageEnrollmentTokens(role string) bool {
	return role == "admin" || role == "operator"
}
