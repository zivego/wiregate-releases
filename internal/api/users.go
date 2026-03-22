package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
)

type userResponse struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Role            string `json:"role"`
	ThemePreference string `json:"theme_preference"`
	CreatedAt       string `json:"created_at"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type resetUserPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

type changeOwnPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type passwordActionResponse struct {
	UserID string `json:"user_id"`
}

type updateOwnPreferencesRequest struct {
	ThemePreference string `json:"theme_preference"`
}

type ownPreferencesResponse struct {
	UserID          string `json:"user_id"`
	ThemePreference string `json:"theme_preference"`
}

type mfaTOTPStatusResponse struct {
	Enabled bool `json:"enabled"`
}

type mfaTOTPSetupResponse struct {
	Enabled      bool   `json:"enabled"`
	Secret       string `json:"secret"`
	Provisioning string `json:"provisioning_uri"`
}

type mfaTOTPCodeRequest struct {
	OTPCode string `json:"otp_code"`
}

type userPage struct {
	Users      []userrepo.User
	NextCursor string
}

var errInvalidUserCursor = fmt.Errorf("invalid user cursor")

// handleListUsers handles GET /api/v1/users (admin only).
func (r *Router) handleListUsers(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
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

	page, err := r.listUsersPage(req.Context(), req.URL.Query().Get("cursor"), pageSize)
	if err == errInvalidUserCursor {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid user cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list users error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list users")
		return
	}

	resp := make([]userResponse, 0, len(page.Users))
	for _, u := range page.Users {
		resp = append(resp, userToResponse(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": resp, "next_cursor": page.NextCursor})
}

func (r *Router) listUsersPage(ctx context.Context, cursor string, limit int) (userPage, error) {
	filter := userrepo.ListFilter{Limit: limit}
	if strings.TrimSpace(cursor) != "" {
		cursorTime, cursorID, err := decodeUserCursor(cursor)
		if err != nil {
			return userPage{}, errInvalidUserCursor
		}
		filter.CursorTime = cursorTime
		filter.CursorID = cursorID
	}
	users, hasMore, err := r.users.ListPage(ctx, filter)
	if err != nil {
		return userPage{}, err
	}
	page := userPage{Users: users}
	if hasMore && len(users) > 0 {
		page.NextCursor = encodeUserCursor(users[len(users)-1])
	}
	return page, nil
}

func encodeUserCursor(user userrepo.User) string {
	payload, _ := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}{
		CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
		ID:        user.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeUserCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", err
	}
	var payload struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return time.Time{}, "", err
	}
	if payload.CreatedAt == "" || payload.ID == "" {
		return time.Time{}, "", fmt.Errorf("cursor is incomplete")
	}
	createdAt, err := time.Parse(time.RFC3339, payload.CreatedAt)
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, payload.ID, nil
}

// handleCreateUser handles POST /api/v1/users (admin only).
func (r *Router) handleCreateUser(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.create") {
		return
	}

	var body createUserRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	body.Email = strings.TrimSpace(body.Email)
	body.Password = strings.TrimSpace(body.Password)
	body.Role = strings.TrimSpace(body.Role)

	if body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "email and password are required")
		return
	}
	if !isValidUserRole(body.Role) {
		writeError(w, http.StatusBadRequest, "validation_failed", "role must be admin, operator, or readonly")
		return
	}

	existing, err := r.users.FindByEmail(req.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to check user")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "conflict", "email already exists")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to hash password")
		return
	}

	id, err := newUserID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate id")
		return
	}

	u := userrepo.User{
		ID:           id,
		Email:        body.Email,
		Role:         body.Role,
		PasswordHash: hash,
	}
	if err := r.users.Insert(req.Context(), u); err != nil {
		r.logger.Printf("create user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create user")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.create",
		ResourceType: "user",
		ResourceID:   id,
		Result:       "success",
		Metadata: map[string]any{
			"email": body.Email,
			"role":  body.Role,
		},
	})
	writeJSON(w, http.StatusCreated, userResponse{
		ID:              id,
		Email:           body.Email,
		Role:            body.Role,
		ThemePreference: "light",
	})
}

// handlePatchUser handles PATCH /api/v1/users/{id} (admin only).
func (r *Router) handlePatchUser(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.update") {
		return
	}

	userID, action, valid := parseUserAction(req.URL.Path)
	if !valid || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	var body updateUserRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.update",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "invalid_request_body",
			},
		})
		writeDecodeError(w, err)
		return
	}
	body.Email = strings.TrimSpace(body.Email)
	body.Role = strings.TrimSpace(body.Role)

	if body.Email == "" || !isValidUserRole(body.Role) {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.update",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "validation_failed",
			},
		})
		writeError(w, http.StatusBadRequest, "validation_failed", "email and valid role are required")
		return
	}

	target, err := r.users.FindByID(req.Context(), userID)
	if err != nil {
		r.logger.Printf("get user for update error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if target == nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.update",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "not_found",
			},
		})
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	if userID == claims.UserID && body.Role != target.Role {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.update",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "self_role_change_blocked",
			},
		})
		writeError(w, http.StatusConflict, "conflict", "cannot change your own role")
		return
	}

	existing, err := r.users.FindByEmail(req.Context(), body.Email)
	if err != nil {
		r.logger.Printf("find by email for update error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to validate user email")
		return
	}
	if existing != nil && existing.ID != userID {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.update",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "email_conflict",
				"email":  body.Email,
			},
		})
		writeError(w, http.StatusConflict, "conflict", "email already exists")
		return
	}

	if err := r.users.UpdateProfile(req.Context(), userID, body.Email, body.Role); err != nil {
		r.logger.Printf("update user profile error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update user")
		return
	}

	updated, err := r.users.FindByID(req.Context(), userID)
	if err != nil || updated == nil {
		r.logger.Printf("reload updated user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load updated user")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.update",
		ResourceType: "user",
		ResourceID:   userID,
		Result:       "success",
		Metadata: map[string]any{
			"old_email": target.Email,
			"new_email": updated.Email,
			"old_role":  target.Role,
			"new_role":  updated.Role,
		},
	})
	writeJSON(w, http.StatusOK, userToResponse(*updated))
}

// handleUserPost handles POST /api/v1/users/{id}/... (admin only).
func (r *Router) handleUserPost(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}

	userID, action, valid := parseUserAction(req.URL.Path)
	if !valid {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	switch action {
	case "password-reset":
		r.handleResetUserPassword(w, req, claims, userID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (r *Router) handleResetUserPassword(w http.ResponseWriter, req *http.Request, claims auth.Claims, userID string) {
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.password.reset") {
		return
	}

	var body resetUserPasswordRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.reset",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "invalid_request_body",
			},
		})
		writeDecodeError(w, err)
		return
	}
	body.NewPassword = strings.TrimSpace(body.NewPassword)
	if body.NewPassword == "" {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.reset",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "validation_failed",
			},
		})
		writeError(w, http.StatusBadRequest, "validation_failed", "new_password is required")
		return
	}

	target, err := r.users.FindByID(req.Context(), userID)
	if err != nil {
		r.logger.Printf("get user for password reset error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if target == nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.reset",
			ResourceType: "user",
			ResourceID:   userID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "not_found",
			},
		})
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to hash password")
		return
	}
	if err := r.users.UpdatePasswordHashAndMustChangePassword(req.Context(), userID, hash, true); err != nil {
		r.logger.Printf("password reset update hash error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to reset password")
		return
	}
	if err := r.authService.RevokeAllSessionsForUser(req.Context(), userID); err != nil {
		r.logger.Printf("password reset revoke sessions error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke sessions")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.password.reset",
		ResourceType: "user",
		ResourceID:   userID,
		Result:       "success",
		Metadata: map[string]any{
			"email": target.Email,
		},
	})
	writeJSON(w, http.StatusOK, passwordActionResponse{UserID: userID})
}

// handleChangeOwnPassword handles POST /api/v1/users/me/password.
func (r *Router) handleChangeOwnPassword(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.password.change") {
		return
	}

	var body changeOwnPasswordRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.change",
			ResourceType: "user",
			ResourceID:   claims.UserID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "invalid_request_body",
			},
		})
		writeDecodeError(w, err)
		return
	}
	body.CurrentPassword = strings.TrimSpace(body.CurrentPassword)
	body.NewPassword = strings.TrimSpace(body.NewPassword)
	if body.CurrentPassword == "" || body.NewPassword == "" {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.change",
			ResourceType: "user",
			ResourceID:   claims.UserID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "validation_failed",
			},
		})
		writeError(w, http.StatusBadRequest, "validation_failed", "current_password and new_password are required")
		return
	}
	if body.CurrentPassword == body.NewPassword {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.change",
			ResourceType: "user",
			ResourceID:   claims.UserID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "same_password",
			},
		})
		writeError(w, http.StatusConflict, "conflict", "new password must be different from current password")
		return
	}

	currentUser, err := r.users.FindByID(req.Context(), claims.UserID)
	if err != nil {
		r.logger.Printf("get current user for password change error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if currentUser == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentUser.PasswordHash), []byte(body.CurrentPassword)); err != nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			ActorUserID:  claims.UserID,
			Action:       "user.password.change",
			ResourceType: "user",
			ResourceID:   claims.UserID,
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "invalid_current_password",
			},
		})
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid current password")
		return
	}

	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to hash password")
		return
	}
	if err := r.users.UpdatePasswordHashAndMustChangePassword(req.Context(), claims.UserID, hash, false); err != nil {
		r.logger.Printf("update own password hash error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to change password")
		return
	}
	if err := r.authService.RevokeAllSessionsForUser(req.Context(), claims.UserID); err != nil {
		r.logger.Printf("change own password revoke sessions error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke sessions")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.password.change",
		ResourceType: "user",
		ResourceID:   claims.UserID,
		Result:       "success",
	})
	writeJSON(w, http.StatusOK, passwordActionResponse{UserID: claims.UserID})
}

// handleUpdateOwnPreferences handles PATCH /api/v1/users/me/preferences.
func (r *Router) handleUpdateOwnPreferences(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	var body updateOwnPreferencesRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	if body.ThemePreference != "light" && body.ThemePreference != "dark" {
		writeError(w, http.StatusBadRequest, "validation_failed", "theme_preference must be light or dark")
		return
	}

	if err := r.users.UpdateThemePreference(req.Context(), claims.UserID, body.ThemePreference); err != nil {
		r.logger.Printf("update user preferences error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update preferences")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.preferences.update",
		ResourceType: "user",
		ResourceID:   claims.UserID,
		Result:       "success",
		Metadata: map[string]any{
			"theme_preference": body.ThemePreference,
		},
	})
	writeJSON(w, http.StatusOK, ownPreferencesResponse{
		UserID:          claims.UserID,
		ThemePreference: body.ThemePreference,
	})
}

// handleGetOwnTOTPStatus handles GET /api/v1/users/me/mfa/totp.
func (r *Router) handleGetOwnTOTPStatus(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	currentUser, err := r.users.FindByID(req.Context(), claims.UserID)
	if err != nil {
		r.logger.Printf("get mfa status user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if currentUser == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return
	}

	writeJSON(w, http.StatusOK, mfaTOTPStatusResponse{Enabled: currentUser.MFATOTPEnabled})
}

// handleSetupOwnTOTP handles POST /api/v1/users/me/mfa/totp/setup.
func (r *Router) handleSetupOwnTOTP(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.mfa.setup") {
		return
	}

	currentUser, err := r.users.FindByID(req.Context(), claims.UserID)
	if err != nil {
		r.logger.Printf("setup mfa user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if currentUser == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return
	}

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		r.logger.Printf("setup mfa secret error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare mfa setup")
		return
	}
	if err := r.users.SetMFATOTP(req.Context(), claims.UserID, secret, false); err != nil {
		r.logger.Printf("setup mfa persist error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare mfa setup")
		return
	}

	provisioning := auth.TOTPProvisioningURI("WireGate", currentUser.Email, secret)
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.mfa.setup",
		ResourceType: "user",
		ResourceID:   claims.UserID,
		Result:       "success",
	})
	writeJSON(w, http.StatusOK, mfaTOTPSetupResponse{
		Enabled:      false,
		Secret:       secret,
		Provisioning: provisioning,
	})
}

// handleEnableOwnTOTP handles POST /api/v1/users/me/mfa/totp/enable.
func (r *Router) handleEnableOwnTOTP(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.mfa.enable") {
		return
	}

	var body mfaTOTPCodeRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	currentUser, err := r.users.FindByID(req.Context(), claims.UserID)
	if err != nil {
		r.logger.Printf("enable mfa user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if currentUser == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return
	}
	if strings.TrimSpace(currentUser.MFATOTPSecret) == "" {
		writeError(w, http.StatusConflict, "conflict", "mfa setup is required before enabling")
		return
	}
	if !auth.ValidateTOTP(currentUser.MFATOTPSecret, body.OTPCode, time.Now().UTC()) {
		writeError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid mfa code")
		return
	}
	if err := r.users.SetMFATOTP(req.Context(), claims.UserID, currentUser.MFATOTPSecret, true); err != nil {
		r.logger.Printf("enable mfa persist error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to enable mfa")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.mfa.enable",
		ResourceType: "user",
		ResourceID:   claims.UserID,
		Result:       "success",
	})
	writeJSON(w, http.StatusOK, mfaTOTPStatusResponse{Enabled: true})
}

// handleDisableOwnTOTP handles POST /api/v1/users/me/mfa/totp/disable.
func (r *Router) handleDisableOwnTOTP(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.mfa.disable") {
		return
	}

	var body mfaTOTPCodeRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	currentUser, err := r.users.FindByID(req.Context(), claims.UserID)
	if err != nil {
		r.logger.Printf("disable mfa user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if currentUser == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return
	}
	if !currentUser.MFATOTPEnabled {
		writeJSON(w, http.StatusOK, mfaTOTPStatusResponse{Enabled: false})
		return
	}
	if !auth.ValidateTOTP(currentUser.MFATOTPSecret, body.OTPCode, time.Now().UTC()) {
		writeError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid mfa code")
		return
	}

	if err := r.users.SetMFATOTP(req.Context(), claims.UserID, "", false); err != nil {
		r.logger.Printf("disable mfa persist error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to disable mfa")
		return
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.mfa.disable",
		ResourceType: "user",
		ResourceID:   claims.UserID,
		Result:       "success",
	})
	writeJSON(w, http.StatusOK, mfaTOTPStatusResponse{Enabled: false})
}

// handleDeleteUser handles DELETE /api/v1/users/{id} (admin only).
func (r *Router) handleDeleteUser(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "user.delete") {
		return
	}

	id, action, valid := parseUserAction(req.URL.Path)
	if !valid || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if id == claims.UserID {
		writeError(w, http.StatusBadRequest, "validation_failed", "cannot delete your own account")
		return
	}

	target, err := r.users.FindByID(req.Context(), id)
	if err != nil {
		r.logger.Printf("get user for delete error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load user")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}

	if err := r.users.Delete(req.Context(), id); err != nil {
		r.logger.Printf("delete user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete user")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "user.delete",
		ResourceType: "user",
		ResourceID:   id,
		Result:       "success",
		Metadata: map[string]any{
			"email": target.Email,
			"role":  target.Role,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}

func userToResponse(u userrepo.User) userResponse {
	return userResponse{
		ID:              u.ID,
		Email:           u.Email,
		Role:            u.Role,
		ThemePreference: u.ThemePreference,
		CreatedAt:       u.CreatedAt.Format("2006-01-02 15:04"),
	}
}

func parseUserAction(path string) (userID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/users/")
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

func isValidUserRole(role string) bool {
	return role == "admin" || role == "operator" || role == "readonly"
}

func newUserID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16])), nil
}
