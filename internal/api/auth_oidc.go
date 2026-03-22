package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
)

const (
	oidcStateCookieName = "wiregate_oidc_state"
	oidcNonceCookieName = "wiregate_oidc_nonce"
)

// OIDCConfig controls optional OIDC SSO behavior.
type OIDCConfig struct {
	DisplayName      string
	IssuerURL        string
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	Scopes           []string
	AdminGroups      []string
	OperatorGroups   []string
	ReadonlyGroups   []string
	RequiredAdminAMR string
	RequiredAdminACR string
	AutoCreateUsers  bool
}

func (c OIDCConfig) enabled() bool {
	return strings.TrimSpace(c.IssuerURL) != "" &&
		strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.ClientSecret) != "" &&
		strings.TrimSpace(c.RedirectURL) != ""
}

type authProviderInfo struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
	DisplayName string `json:"display_name"`
}

type authProvidersResponse struct {
	Providers []authProviderInfo `json:"providers"`
}

type oidcAuthenticator struct {
	config    OIDCConfig
	roleByGrp map[string]string

	initOnce sync.Once
	initErr  error
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   oauth2.Config
}

type oidcIdentity struct {
	Email   string
	Subject string
	Groups  []string
	AMR     []string
	ACR     string
}

func newOIDCAuthenticator(cfg OIDCConfig) (*oidcAuthenticator, error) {
	if !cfg.enabled() {
		return nil, nil
	}
	roleByGroup := make(map[string]string)
	for _, group := range cfg.AdminGroups {
		normalized := strings.ToLower(strings.TrimSpace(group))
		if normalized != "" {
			roleByGroup[normalized] = "admin"
		}
	}
	for _, group := range cfg.OperatorGroups {
		normalized := strings.ToLower(strings.TrimSpace(group))
		if normalized != "" {
			roleByGroup[normalized] = "operator"
		}
	}
	for _, group := range cfg.ReadonlyGroups {
		normalized := strings.ToLower(strings.TrimSpace(group))
		if normalized != "" {
			roleByGroup[normalized] = "readonly"
		}
	}
	if len(roleByGroup) == 0 {
		return nil, errors.New("oidc group mapping is empty")
	}
	scopes := normalizeOIDCScopes(cfg.Scopes)
	return &oidcAuthenticator{
		config:    cfg,
		roleByGrp: roleByGroup,
		oauth2: oauth2.Config{
			ClientID:     strings.TrimSpace(cfg.ClientID),
			ClientSecret: strings.TrimSpace(cfg.ClientSecret),
			RedirectURL:  strings.TrimSpace(cfg.RedirectURL),
			Scopes:       scopes,
		},
	}, nil
}

func normalizeOIDCScopes(scopes []string) []string {
	hasOpenID := false
	normalized := make([]string, 0, len(scopes)+1)
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if scope == "openid" {
			hasOpenID = true
		}
		normalized = append(normalized, scope)
	}
	if !hasOpenID {
		normalized = append([]string{"openid"}, normalized...)
	}
	return normalized
}

func (a *oidcAuthenticator) ensureInitialized(ctx context.Context) error {
	a.initOnce.Do(func() {
		initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		provider, err := oidc.NewProvider(initCtx, strings.TrimSpace(a.config.IssuerURL))
		if err != nil {
			a.initErr = fmt.Errorf("oidc provider discovery: %w", err)
			return
		}
		a.provider = provider
		a.verifier = provider.Verifier(&oidc.Config{ClientID: a.oauth2.ClientID})
		a.oauth2.Endpoint = provider.Endpoint()
	})
	return a.initErr
}

func (a *oidcAuthenticator) authCodeURL(ctx context.Context, state, nonce string) (string, error) {
	if err := a.ensureInitialized(ctx); err != nil {
		return "", err
	}
	return a.oauth2.AuthCodeURL(state, oidc.Nonce(nonce)), nil
}

func (a *oidcAuthenticator) exchangeCode(ctx context.Context, code, nonce string) (oidcIdentity, error) {
	if err := a.ensureInitialized(ctx); err != nil {
		return oidcIdentity{}, err
	}
	token, err := a.oauth2.Exchange(ctx, code)
	if err != nil {
		return oidcIdentity{}, fmt.Errorf("oidc exchange code: %w", err)
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if strings.TrimSpace(rawIDToken) == "" {
		return oidcIdentity{}, errors.New("missing id_token in provider response")
	}
	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return oidcIdentity{}, fmt.Errorf("oidc verify id_token: %w", err)
	}
	if strings.TrimSpace(nonce) != "" && idToken.Nonce != nonce {
		return oidcIdentity{}, errors.New("invalid oidc nonce")
	}

	var claims struct {
		Email             string   `json:"email"`
		PreferredUsername string   `json:"preferred_username"`
		Subject           string   `json:"sub"`
		Groups            []string `json:"groups"`
		AMR               []string `json:"amr"`
		ACR               string   `json:"acr"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return oidcIdentity{}, fmt.Errorf("oidc parse claims: %w", err)
	}
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		email = strings.TrimSpace(claims.PreferredUsername)
	}
	if email == "" {
		return oidcIdentity{}, errors.New("oidc email claim is required")
	}
	return oidcIdentity{
		Email:   strings.ToLower(email),
		Subject: strings.TrimSpace(claims.Subject),
		Groups:  claims.Groups,
		AMR:     claims.AMR,
		ACR:     strings.TrimSpace(claims.ACR),
	}, nil
}

func (a *oidcAuthenticator) resolveRole(identity oidcIdentity) (string, error) {
	for _, group := range identity.Groups {
		if role, ok := a.roleByGrp[strings.ToLower(strings.TrimSpace(group))]; ok {
			return role, nil
		}
	}
	return "", errors.New("no mapped OIDC group for wiregate role")
}

func (r *Router) handleAuthProviders(w http.ResponseWriter, _ *http.Request) {
	providers := []authProviderInfo{
		{
			ID:          "local",
			Type:        "password",
			Enabled:     true,
			DisplayName: "Local account",
		},
	}
	if r.oidc != nil {
		providers = append(providers, authProviderInfo{
			ID:          "oidc",
			Type:        "oidc",
			Enabled:     true,
			DisplayName: r.oidc.config.DisplayName,
		})
	} else {
		providers = append(providers, authProviderInfo{
			ID:          "oidc",
			Type:        "oidc",
			Enabled:     false,
			DisplayName: "Single Sign-On",
		})
	}
	writeJSON(w, http.StatusOK, authProvidersResponse{Providers: providers})
}

func (r *Router) handleOIDCStart(w http.ResponseWriter, req *http.Request) {
	if r.oidc == nil {
		writeError(w, http.StatusNotFound, "not_found", "oidc provider is not configured")
		return
	}
	state, err := randomHexToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to initialize oidc flow")
		return
	}
	nonce, err := randomHexToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to initialize oidc flow")
		return
	}
	redirectURL, err := r.oidc.authCodeURL(req.Context(), state, nonce)
	if err != nil {
		r.logger.Printf("oidc start error: %v", err)
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "oidc provider is unavailable")
		return
	}

	r.setOIDCCallbackCookie(w, oidcStateCookieName, state, 300)
	r.setOIDCCallbackCookie(w, oidcNonceCookieName, nonce, 300)
	http.Redirect(w, req, redirectURL, http.StatusFound)
}

func (r *Router) handleOIDCCallback(w http.ResponseWriter, req *http.Request) {
	if r.oidc == nil {
		writeError(w, http.StatusNotFound, "not_found", "oidc provider is not configured")
		return
	}
	defer func() {
		r.clearOIDCCallbackCookie(w, oidcStateCookieName)
		r.clearOIDCCallbackCookie(w, oidcNonceCookieName)
	}()

	state := strings.TrimSpace(req.URL.Query().Get("state"))
	code := strings.TrimSpace(req.URL.Query().Get("code"))
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "oidc callback requires code and state")
		return
	}

	stateCookie, err := req.Cookie(oidcStateCookieName)
	if err != nil || strings.TrimSpace(stateCookie.Value) == "" || stateCookie.Value != state {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid oidc callback state")
		return
	}
	nonceCookie, err := req.Cookie(oidcNonceCookieName)
	if err != nil || strings.TrimSpace(nonceCookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid oidc callback nonce")
		return
	}

	identity, err := r.oidc.exchangeCode(req.Context(), code, nonceCookie.Value)
	if err != nil {
		r.logger.Printf("oidc exchange error: %v", err)
		r.recordAuditEvent(req.Context(), audit.Event{
			Action:       "auth.login_oidc",
			ResourceType: "session",
			Result:       "failure",
			Metadata: map[string]any{
				"reason": "exchange_failed",
			},
		})
		writeError(w, http.StatusUnauthorized, "unauthorized", "oidc login failed")
		return
	}

	role, err := r.oidc.resolveRole(identity)
	if err != nil {
		r.recordAuditEvent(req.Context(), audit.Event{
			Action:       "auth.login_oidc",
			ResourceType: "session",
			Result:       "failure",
			Metadata: map[string]any{
				"email":  identity.Email,
				"reason": "role_mapping_failed",
			},
		})
		writeError(w, http.StatusForbidden, "forbidden", "oidc identity does not satisfy role policy")
		return
	}
	if role == "admin" && r.securityService != nil {
		if err := r.securityService.ValidateAdminOIDCClaims(req.Context(), identity.AMR, identity.ACR); err != nil {
			r.recordAuditEvent(req.Context(), audit.Event{
				Action:       "auth.login_oidc",
				ResourceType: "session",
				Result:       "failure",
				Metadata: map[string]any{
					"email":  identity.Email,
					"reason": "admin_claims_policy_failed",
				},
			})
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
	}

	user, err := r.users.FindByEmail(req.Context(), identity.Email)
	if err != nil {
		r.logger.Printf("oidc find user error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
		return
	}
	if user == nil {
		if !r.oidc.config.AutoCreateUsers {
			writeError(w, http.StatusForbidden, "forbidden", "oidc account is not provisioned")
			return
		}
		passwordSeed, err := randomHexToken(32)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
			return
		}
		passwordHash, err := auth.HashPassword(passwordSeed)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
			return
		}
		if err := r.users.Insert(req.Context(), userrepo.User{
			ID:                 uuid.NewString(),
			Email:              identity.Email,
			Role:               role,
			PasswordHash:       passwordHash,
			MustChangePassword: false,
			ThemePreference:    "light",
		}); err != nil {
			r.logger.Printf("oidc create user error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
			return
		}
		user, err = r.users.FindByEmail(req.Context(), identity.Email)
		if err != nil || user == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
			return
		}
	} else if user.Role != role {
		if err := r.users.UpdateProfile(req.Context(), user.ID, user.Email, role); err != nil {
			r.logger.Printf("oidc update role error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
			return
		}
		user.Role = role
	}

	claims, rawToken, err := r.authService.CreateSessionForUserWithMetadata(req.Context(), user, sessionMetadataFromRequest(req, "oidc"))
	if err != nil {
		r.logger.Printf("oidc create session error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "oidc login failed")
		return
	}
	r.setSessionCookie(w, rawToken, claims.ExpiresAt)
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "auth.login_oidc",
		ResourceType: "session",
		ResourceID:   claims.SessionID,
		Result:       "success",
		Metadata: map[string]any{
			"email":         claims.Email,
			"role":          claims.Role,
			"subject":       identity.Subject,
			"auth_provider": claims.AuthProvider,
		},
	})

	target := "/dashboard"
	if claims.MustChangePassword {
		target = "/account"
	}
	http.Redirect(w, req, target, http.StatusFound)
}

func randomHexToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (r *Router) setOIDCCallbackCookie(w http.ResponseWriter, name, value string, ttlSeconds int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/v1/auth/oidc/callback",
		MaxAge:   ttlSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !r.cookieInsecure,
	})
}

func (r *Router) clearOIDCCallbackCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/api/v1/auth/oidc/callback",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !r.cookieInsecure,
	})
}
