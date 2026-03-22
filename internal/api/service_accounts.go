package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
)

type serviceAccountResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

type serviceAccountKeyResponse struct {
	ID               string `json:"id"`
	ServiceAccountID string `json:"service_account_id"`
	Name             string `json:"name"`
	KeyPrefix        string `json:"key_prefix"`
	Status           string `json:"status"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	CreatedAt        string `json:"created_at"`
	RevokedAt        string `json:"revoked_at,omitempty"`
	LastUsedAt       string `json:"last_used_at,omitempty"`
	Token            string `json:"token,omitempty"`
}

type createServiceAccountRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type createServiceAccountKeyRequest struct {
	Name       string `json:"name"`
	TTLMinutes int    `json:"ttl_minutes,omitempty"`
}

func (r *Router) handleListServiceAccounts(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if r.serviceAccounts == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service account manager is not configured")
		return
	}
	accounts, err := r.serviceAccounts.ListServiceAccounts(req.Context())
	if err != nil {
		r.logger.Printf("list service accounts error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list service accounts")
		return
	}
	resp := make([]serviceAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, serviceAccountResponse{
			ID:         account.ID,
			Name:       account.Name,
			Role:       account.Role,
			Status:     account.Status,
			CreatedAt:  account.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:  account.UpdatedAt.UTC().Format(time.RFC3339),
			LastUsedAt: formatOptionalTime(account.LastUsedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"service_accounts": resp})
}

func (r *Router) handleCreateServiceAccount(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "service_account.create") {
		return
	}
	if r.serviceAccounts == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service account manager is not configured")
		return
	}

	var body createServiceAccountRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	account, err := r.serviceAccounts.CreateServiceAccount(req.Context(), body.Name, body.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "service_account.create",
		ResourceType: "service_account",
		ResourceID:   account.ID,
		Result:       "success",
		Metadata: map[string]any{
			"name": account.Name,
			"role": account.Role,
		},
	})
	writeJSON(w, http.StatusCreated, serviceAccountResponse{
		ID:         account.ID,
		Name:       account.Name,
		Role:       account.Role,
		Status:     account.Status,
		CreatedAt:  account.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  account.UpdatedAt.UTC().Format(time.RFC3339),
		LastUsedAt: formatOptionalTime(account.LastUsedAt),
	})
}

func (r *Router) handleServiceAccountGet(w http.ResponseWriter, req *http.Request) {
	accountID, keyID, action, ok := parseServiceAccountPath(req.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if keyID == "" && action == "keys" {
		r.handleListServiceAccountKeys(w, req, accountID)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}

func (r *Router) handleServiceAccountPost(w http.ResponseWriter, req *http.Request) {
	accountID, keyID, action, ok := parseServiceAccountPath(req.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if keyID == "" && action == "keys" {
		r.handleCreateServiceAccountKey(w, req, accountID)
		return
	}
	if action == "revoke" && keyID != "" {
		r.handleRevokeServiceAccountKey(w, req, accountID, keyID)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
}

func (r *Router) handleListServiceAccountKeys(w http.ResponseWriter, req *http.Request, accountID string) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if r.serviceAccounts == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service account manager is not configured")
		return
	}
	keys, err := r.serviceAccounts.ListAPIKeys(req.Context(), accountID)
	if err != nil {
		r.logger.Printf("list service account keys error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list service account keys")
		return
	}
	resp := make([]serviceAccountKeyResponse, 0, len(keys))
	for _, key := range keys {
		resp = append(resp, serviceAccountKeyResponse{
			ID:               key.ID,
			ServiceAccountID: key.ServiceAccountID,
			Name:             key.Name,
			KeyPrefix:        key.KeyPrefix,
			Status:           key.Status,
			ExpiresAt:        formatOptionalTime(key.ExpiresAt),
			CreatedAt:        key.CreatedAt.UTC().Format(time.RFC3339),
			RevokedAt:        formatOptionalTime(key.RevokedAt),
			LastUsedAt:       formatOptionalTime(key.LastUsedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": resp})
}

func (r *Router) handleCreateServiceAccountKey(w http.ResponseWriter, req *http.Request, accountID string) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "service_account.key.create") {
		return
	}
	if r.serviceAccounts == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service account manager is not configured")
		return
	}

	var body createServiceAccountKeyRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	ttl := time.Duration(body.TTLMinutes) * time.Minute
	created, err := r.serviceAccounts.CreateAPIKey(req.Context(), accountID, body.Name, ttl)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "service_account.key.create",
		ResourceType: "service_account",
		ResourceID:   accountID,
		Result:       "success",
		Metadata: map[string]any{
			"key_id": created.Key.ID,
			"name":   created.Key.Name,
		},
	})
	writeJSON(w, http.StatusCreated, serviceAccountKeyResponse{
		ID:               created.Key.ID,
		ServiceAccountID: created.Key.ServiceAccountID,
		Name:             created.Key.Name,
		KeyPrefix:        created.Key.KeyPrefix,
		Status:           created.Key.Status,
		ExpiresAt:        formatOptionalTime(created.Key.ExpiresAt),
		CreatedAt:        created.Key.CreatedAt.UTC().Format(time.RFC3339),
		RevokedAt:        formatOptionalTime(created.Key.RevokedAt),
		LastUsedAt:       formatOptionalTime(created.Key.LastUsedAt),
		Token:            created.Raw,
	})
}

func (r *Router) handleRevokeServiceAccountKey(w http.ResponseWriter, req *http.Request, accountID, keyID string) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "service_account.key.revoke") {
		return
	}
	if r.serviceAccounts == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service account manager is not configured")
		return
	}
	if err := r.serviceAccounts.RevokeAPIKey(req.Context(), accountID, keyID); err != nil {
		r.logger.Printf("revoke service account key error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke key")
		return
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "service_account.key.revoke",
		ResourceType: "service_account",
		ResourceID:   accountID,
		Result:       "success",
		Metadata: map[string]any{
			"key_id": keyID,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}

func parseServiceAccountPath(path string) (accountID string, keyID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/service-accounts/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", "", false
	}
	if parts[1] != "keys" {
		return "", "", "", false
	}
	switch len(parts) {
	case 2:
		return parts[0], "", "keys", true
	case 4:
		if strings.TrimSpace(parts[2]) == "" || strings.TrimSpace(parts[3]) == "" {
			return "", "", "", false
		}
		return parts[0], parts[2], parts[3], true
	default:
		return "", "", "", false
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
