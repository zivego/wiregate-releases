package api

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
)

// loginRequest is the JSON body for POST /api/v1/sessions.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OTPCode  string `json:"otp_code,omitempty"`
}

// sessionResponse is the JSON body returned on successful login or session inspect.
type sessionResponse struct {
	SessionID          string `json:"session_id"`
	UserID             string `json:"user_id"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
	ThemePreference    string `json:"theme_preference"`
	AuthProvider       string `json:"auth_provider"`
	LastSeenAt         string `json:"last_seen_at"`
	SourceIP           string `json:"source_ip,omitempty"`
	UserAgent          string `json:"user_agent,omitempty"`
	IssuedAt           string `json:"issued_at"`
	ExpiresAt          string `json:"expires_at"`
}

type sessionInventoryItemResponse struct {
	SessionID    string `json:"session_id"`
	UserID       string `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	AuthProvider string `json:"auth_provider"`
	Current      bool   `json:"current"`
	IssuedAt     string `json:"issued_at"`
	ExpiresAt    string `json:"expires_at"`
	LastSeenAt   string `json:"last_seen_at"`
	SourceIP     string `json:"source_ip,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
}

// rateLimiter is the interface used by the router for all rate limiting.
type rateLimiter interface {
	allow(key string) bool
}

// memoryRateLimiter is the in-memory sliding window implementation (used as fallback).
type memoryRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	maxN     int
	window   time.Duration
}

func newRateLimiter(maxN int, window time.Duration) rateLimiter {
	return &memoryRateLimiter{
		attempts: make(map[string][]time.Time),
		maxN:     maxN,
		window:   window,
	}
}

func (rl *memoryRateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	prev := rl.attempts[key]
	var recent []time.Time
	for _, t := range prev {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= rl.maxN {
		rl.attempts[key] = recent
		return false
	}
	rl.attempts[key] = append(recent, now)
	return true
}

// handleLogin handles POST /api/v1/sessions.
func (r *Router) handleLogin(w http.ResponseWriter, req *http.Request) {
	var body loginRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	body.Email = strings.TrimSpace(body.Email)
	body.Password = strings.TrimSpace(body.Password)
	if body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "email and password are required")
		return
	}

	if !r.loginLimiter.allow(body.Email) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts")
		return
	}

	claims, rawToken, err := r.authService.LoginWithMetadata(req.Context(), body.Email, body.Password, sessionMetadataFromRequest(req, "password", body.OTPCode))
	if errors.Is(err, auth.ErrInvalidCredentials) {
		r.recordAuditEvent(req.Context(), audit.Event{
			Action:       "auth.login",
			ResourceType: "session",
			Result:       "failure",
			Metadata: map[string]any{
				"email": body.Email,
			},
		})
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	if errors.Is(err, auth.ErrMFARequired) {
		writeError(w, http.StatusUnauthorized, "mfa_required", "mfa code is required")
		return
	}
	if errors.Is(err, auth.ErrInvalidMFACode) {
		writeError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid mfa code")
		return
	}
	if err != nil {
		r.logger.Printf("login error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "login failed")
		return
	}

	r.setSessionCookie(w, rawToken, claims.ExpiresAt)
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "auth.login",
		ResourceType: "session",
		ResourceID:   claims.SessionID,
		Result:       "success",
		Metadata: map[string]any{
			"email":         claims.Email,
			"role":          claims.Role,
			"auth_provider": claims.AuthProvider,
		},
	})
	writeJSON(w, http.StatusOK, sessionResponse{
		SessionID:          claims.SessionID,
		UserID:             claims.UserID,
		Email:              claims.Email,
		Role:               claims.Role,
		MustChangePassword: claims.MustChangePassword,
		ThemePreference:    claims.ThemePreference,
		AuthProvider:       claims.AuthProvider,
		LastSeenAt:         claims.LastSeenAt.Format(time.RFC3339),
		SourceIP:           claims.SourceIP,
		UserAgent:          claims.UserAgent,
		IssuedAt:           claims.IssuedAt.Format(time.RFC3339),
		ExpiresAt:          claims.ExpiresAt.Format(time.RFC3339),
	})
}

func (r *Router) handleListSessions(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
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

	page, err := r.authService.ListSessionsPage(req.Context(), claims.UserID, claims.Role, claims.RawSessionToken, req.URL.Query().Get("cursor"), pageSize)
	if errors.Is(err, auth.ErrInvalidSessionCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid session cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list sessions error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list sessions")
		return
	}

	resp := make([]sessionInventoryItemResponse, 0, len(page.Sessions))
	for _, session := range page.Sessions {
		resp = append(resp, sessionInventoryItemResponse{
			SessionID:    session.SessionID,
			UserID:       session.UserID,
			Email:        session.Email,
			Role:         session.Role,
			AuthProvider: session.AuthProvider,
			Current:      session.Current,
			IssuedAt:     session.IssuedAt.Format(time.RFC3339),
			ExpiresAt:    session.ExpiresAt.Format(time.RFC3339),
			LastSeenAt:   session.LastSeenAt.Format(time.RFC3339),
			SourceIP:     session.SourceIP,
			UserAgent:    session.UserAgent,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": resp, "next_cursor": page.NextCursor})
}

// handleSessionCurrent handles GET /api/v1/sessions/current.
func (r *Router) handleSessionCurrent(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		SessionID:          claims.SessionID,
		UserID:             claims.UserID,
		Email:              claims.Email,
		Role:               claims.Role,
		MustChangePassword: claims.MustChangePassword,
		ThemePreference:    claims.ThemePreference,
		AuthProvider:       claims.AuthProvider,
		LastSeenAt:         claims.LastSeenAt.Format(time.RFC3339),
		SourceIP:           claims.SourceIP,
		UserAgent:          claims.UserAgent,
		IssuedAt:           claims.IssuedAt.Format(time.RFC3339),
		ExpiresAt:          claims.ExpiresAt.Format(time.RFC3339),
	})
}

// handleLogout handles DELETE /api/v1/sessions/current.
func (r *Router) handleLogout(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	if err := r.authService.Logout(req.Context(), claims.RawSessionToken); err != nil {
		r.logger.Printf("logout error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "logout failed")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "auth.logout",
		ResourceType: "session",
		ResourceID:   claims.SessionID,
		Result:       "success",
		Metadata: map[string]any{
			"email":         claims.Email,
			"role":          claims.Role,
			"auth_provider": claims.AuthProvider,
		},
	})
	r.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleSessionPost(w http.ResponseWriter, req *http.Request) {
	sessionID, action, valid := parseSessionAction(req.URL.Path)
	if !valid || action != "revoke" {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}

	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	target, err := r.authService.RevokeSessionByPublicID(req.Context(), claims.UserID, claims.Role, sessionID)
	if errors.Is(err, auth.ErrInvalidSession) {
		writeError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if err != nil {
		r.logger.Printf("revoke session error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to revoke session")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "session.revoke",
		ResourceType: "session",
		ResourceID:   target.SessionID,
		Result:       "success",
		Metadata: map[string]any{
			"user_id":       target.UserID,
			"email":         target.Email,
			"role":          target.Role,
			"auth_provider": target.AuthProvider,
		},
	})
	if target.SessionID == claims.SessionID {
		r.clearSessionCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

// authenticate extracts and validates the session token, writing a 401 if invalid.
// Returns (Claims, true) on success, (Claims{}, false) on failure.
func (r *Router) authenticate(w http.ResponseWriter, req *http.Request) (auth.Claims, bool) {
	rawToken, ok := extractToken(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return auth.Claims{}, false
	}
	if hasBearerAuth(req) && r.apiKeyAuth != nil {
		claims, matched, err := r.apiKeyAuth.AuthenticateAPIKey(req.Context(), rawToken)
		if err != nil {
			r.logger.Printf("api key auth error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to validate credentials")
			return auth.Claims{}, false
		}
		if matched {
			return claims, true
		}
	}

	claims, err := r.authService.ValidateToken(req.Context(), rawToken)
	if errors.Is(err, auth.ErrInvalidSession) || err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "no valid session")
		return auth.Claims{}, false
	}
	if err := validateCookieAuthRequest(req); err != nil {
		writeError(w, http.StatusForbidden, "csrf_protection", err.Error())
		return auth.Claims{}, false
	}
	if claims.MustChangePassword && !isPasswordChangeAllowedPath(req.Method, req.URL.Path) {
		writeError(w, http.StatusForbidden, "password_change_required", "password change required before accessing this endpoint")
		return auth.Claims{}, false
	}
	lastSeenAt, err := r.authService.TouchSession(req.Context(), claims.RawSessionToken, claims.LastSeenAt, time.Now().UTC())
	if err != nil {
		r.logger.Printf("touch session error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to validate session")
		return auth.Claims{}, false
	}
	claims.LastSeenAt = lastSeenAt
	return claims, true
}

func validateCookieAuthRequest(req *http.Request) error {
	if isSafeMethod(req.Method) || hasBearerAuth(req) {
		return nil
	}
	cookie, err := req.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil
	}
	if origin := strings.TrimSpace(req.Header.Get("Origin")); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("cross-site request blocked")
		}
		scheme := requestScheme(req)
		if !sameOriginHostPort(parsed, req, scheme) {
			return errors.New("cross-site request blocked")
		}
	}
	if strings.EqualFold(strings.TrimSpace(req.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return errors.New("cross-site request blocked")
	}
	return nil
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func hasBearerAuth(req *http.Request) bool {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if len(authHeader) < len("Bearer ")+1 {
		return false
	}
	return strings.EqualFold(authHeader[:len("Bearer ")], "Bearer ")
}

func requestScheme(req *http.Request) string {
	if proto := strings.TrimSpace(req.Header.Get("X-Forwarded-Proto")); proto != "" {
		if comma := strings.Index(proto, ","); comma >= 0 {
			proto = strings.TrimSpace(proto[:comma])
		}
		return strings.ToLower(proto)
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

func sameOriginHostPort(origin *url.URL, req *http.Request, scheme string) bool {
	requestURL := &url.URL{Scheme: scheme, Host: req.Host}
	if !strings.EqualFold(origin.Hostname(), requestURL.Hostname()) {
		return false
	}
	return effectivePort(origin) == effectivePort(requestURL)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

func isPasswordChangeAllowedPath(method, path string) bool {
	if method == http.MethodGet && path == "/api/v1/sessions/current" {
		return true
	}
	if method == http.MethodDelete && path == "/api/v1/sessions/current" {
		return true
	}
	return method == http.MethodPost && path == "/api/v1/users/me/password"
}

func sessionMetadataFromRequest(req *http.Request, authProvider string, mfaCode ...string) auth.SessionMetadata {
	code := ""
	if len(mfaCode) > 0 {
		code = strings.TrimSpace(mfaCode[0])
	}
	return auth.SessionMetadata{
		AuthProvider: authProvider,
		SourceIP:     requestSourceIP(req),
		UserAgent:    strings.TrimSpace(req.Header.Get("User-Agent")),
		MFATOTPCode:  code,
	}
}

func requestSourceIP(req *http.Request) string {
	if forwardedFor := strings.TrimSpace(req.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(req.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(req.RemoteAddr)
}

func parseSessionAction(path string) (sessionID, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/sessions/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

// setSessionCookie writes the session cookie to the response.
func (r *Router) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   !r.cookieInsecure,
	}
	http.SetCookie(w, cookie)
}

// clearSessionCookie removes the session cookie from the browser.
func (r *Router) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// writeError writes a JSON error response in the standard error envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
