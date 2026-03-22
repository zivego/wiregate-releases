package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/zivego/wiregate/internal/persistence/sessionrepo"
	"github.com/zivego/wiregate/internal/persistence/userrepo"
)

// ErrInvalidCredentials is returned when email or password do not match.
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrMFARequired = errors.New("mfa required")
var ErrInvalidMFACode = errors.New("invalid mfa code")

// ErrInvalidSession is returned when the bearer token is missing, expired, or revoked.
var ErrInvalidSession = errors.New("invalid session")
var ErrInvalidSessionCursor = errors.New("invalid session cursor")

// Claims carries the authenticated identity for a single request.
type Claims struct {
	UserID             string
	Email              string
	Role               string
	MustChangePassword bool
	ThemePreference    string
	SessionID          string
	RawSessionToken    string
	AuthProvider       string
	LastSeenAt         time.Time
	SourceIP           string
	UserAgent          string
	IssuedAt           time.Time
	ExpiresAt          time.Time
}

type SessionMetadata struct {
	AuthProvider string
	SourceIP     string
	UserAgent    string
	MFATOTPCode  string
}

type SessionInventoryItem struct {
	SessionID    string
	UserID       string
	Email        string
	Role         string
	AuthProvider string
	Current      bool
	IssuedAt     time.Time
	ExpiresAt    time.Time
	LastSeenAt   time.Time
	SourceIP     string
	UserAgent    string
}

type SessionCursorPage struct {
	Sessions   []SessionInventoryItem
	NextCursor string
}

// Service handles session-based authentication and token lifecycle.
type Service struct {
	users    *userrepo.Repo
	sessions *sessionrepo.Repo
	ttl      time.Duration
	idleTTL  time.Duration
}

// NewService creates a Service with the given repositories and session TTL.
func NewService(users *userrepo.Repo, sessions *sessionrepo.Repo, ttl time.Duration, idleTTL ...time.Duration) *Service {
	resolvedIdleTTL := 30 * time.Minute
	if len(idleTTL) > 0 && idleTTL[0] > 0 {
		resolvedIdleTTL = idleTTL[0]
	}
	return &Service{users: users, sessions: sessions, ttl: ttl, idleTTL: resolvedIdleTTL}
}

// Login validates email+password, creates a session, and returns Claims plus the raw bearer token.
func (s *Service) Login(ctx context.Context, email, password string) (Claims, string, error) {
	return s.LoginWithMetadata(ctx, email, password, SessionMetadata{})
}

func (s *Service) LoginWithMetadata(ctx context.Context, email, password string, metadata SessionMetadata) (Claims, string, error) {
	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return Claims{}, "", fmt.Errorf("auth login: %w", err)
	}
	if user == nil {
		return Claims{}, "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return Claims{}, "", ErrInvalidCredentials
	}
	if user.MFATOTPEnabled {
		code := normalizeOTPCode(metadata.MFATOTPCode)
		if code == "" {
			return Claims{}, "", ErrMFARequired
		}
		if !ValidateTOTP(user.MFATOTPSecret, code, time.Now().UTC()) {
			return Claims{}, "", ErrInvalidMFACode
		}
	}

	return s.CreateSessionForUserWithMetadata(ctx, user, metadata)
}

// CreateSessionForUser creates a new session for an already authenticated user.
func (s *Service) CreateSessionForUser(ctx context.Context, user *userrepo.User) (Claims, string, error) {
	return s.CreateSessionForUserWithMetadata(ctx, user, SessionMetadata{})
}

func (s *Service) CreateSessionForUserWithMetadata(ctx context.Context, user *userrepo.User, metadata SessionMetadata) (Claims, string, error) {
	if user == nil {
		return Claims{}, "", ErrInvalidCredentials
	}
	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return Claims{}, "", fmt.Errorf("auth generate token: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.ttl)
	publicID := uuid.NewString()

	sess := sessionrepo.Session{
		ID:           tokenHash,
		PublicID:     publicID,
		UserID:       user.ID,
		AuthProvider: metadata.AuthProvider,
		IssuedAt:     now,
		ExpiresAt:    expiresAt,
		LastSeenAt:   now,
		SourceIP:     metadata.SourceIP,
		UserAgent:    metadata.UserAgent,
	}
	if err := s.sessions.Insert(ctx, sess); err != nil {
		return Claims{}, "", fmt.Errorf("auth create session: %w", err)
	}

	return Claims{
		UserID:             user.ID,
		Email:              user.Email,
		Role:               user.Role,
		MustChangePassword: user.MustChangePassword,
		ThemePreference:    user.ThemePreference,
		SessionID:          publicID,
		RawSessionToken:    rawToken,
		AuthProvider:       normalizeSessionAuthProvider(sess.AuthProvider),
		LastSeenAt:         now,
		SourceIP:           metadata.SourceIP,
		UserAgent:          metadata.UserAgent,
		IssuedAt:           now,
		ExpiresAt:          expiresAt,
	}, rawToken, nil
}

// ValidateToken looks up the hashed token and returns Claims if the session is active.
func (s *Service) ValidateToken(ctx context.Context, rawToken string) (Claims, error) {
	tokenHash := hashToken(rawToken)
	sess, err := s.sessions.FindActive(ctx, tokenHash, time.Now().UTC().Add(-s.idleTTL))
	if err != nil {
		return Claims{}, fmt.Errorf("auth validate token: %w", err)
	}
	if sess == nil {
		return Claims{}, ErrInvalidSession
	}

	user, err := s.users.FindByID(ctx, sess.UserID)
	if err != nil {
		return Claims{}, fmt.Errorf("auth fetch user: %w", err)
	}
	if user == nil {
		return Claims{}, ErrInvalidSession
	}

	return Claims{
		UserID:             user.ID,
		Email:              user.Email,
		Role:               user.Role,
		MustChangePassword: user.MustChangePassword,
		ThemePreference:    user.ThemePreference,
		SessionID:          sess.PublicID,
		RawSessionToken:    rawToken,
		AuthProvider:       sess.AuthProvider,
		LastSeenAt:         sess.LastSeenAt,
		SourceIP:           sess.SourceIP,
		UserAgent:          sess.UserAgent,
		IssuedAt:           sess.IssuedAt,
		ExpiresAt:          sess.ExpiresAt,
	}, nil
}

// Logout revokes the session associated with rawToken.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	tokenHash := hashToken(rawToken)
	if err := s.sessions.Revoke(ctx, tokenHash, "logout"); err != nil {
		return fmt.Errorf("auth logout: %w", err)
	}
	return nil
}

// RevokeAllSessionsForUser revokes every active session belonging to userID.
func (s *Service) RevokeAllSessionsForUser(ctx context.Context, userID string) error {
	if err := s.sessions.RevokeAllForUser(ctx, userID, "user_security_change"); err != nil {
		return fmt.Errorf("auth revoke all sessions: %w", err)
	}
	return nil
}

func (s *Service) TouchSession(ctx context.Context, rawToken string, lastSeenAt, now time.Time) (time.Time, error) {
	if now.Sub(lastSeenAt) < 5*time.Minute {
		return lastSeenAt, nil
	}
	if err := s.sessions.Touch(ctx, hashToken(rawToken), now); err != nil {
		return lastSeenAt, fmt.Errorf("auth touch session: %w", err)
	}
	return now, nil
}

func (s *Service) ListSessions(ctx context.Context, actorUserID, actorRole, currentRawToken string) ([]SessionInventoryItem, error) {
	page, err := s.ListSessionsPage(ctx, actorUserID, actorRole, currentRawToken, "", 200)
	if err != nil {
		return nil, err
	}
	return page.Sessions, nil
}

func (s *Service) ListSessionsPage(ctx context.Context, actorUserID, actorRole, currentRawToken, cursor string, limit int) (SessionCursorPage, error) {
	filter := sessionrepo.ListFilter{
		IdleCutoff: time.Now().UTC().Add(-s.idleTTL),
		Limit:      limit,
	}
	if actorRole != "admin" {
		filter.UserID = actorUserID
	}
	if cursor != "" {
		lastSeenAt, issuedAt, sessionID, err := decodeSessionCursor(cursor)
		if err != nil {
			return SessionCursorPage{}, ErrInvalidSessionCursor
		}
		filter.CursorSeen = lastSeenAt
		filter.CursorIssue = issuedAt
		filter.CursorID = sessionID
	}
	sessions, hasMore, err := s.sessions.ListPage(ctx, filter)
	if err != nil {
		return SessionCursorPage{}, fmt.Errorf("auth list sessions: %w", err)
	}

	currentHash := hashToken(currentRawToken)
	out := make([]SessionInventoryItem, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, SessionInventoryItem{
			SessionID:    session.PublicID,
			UserID:       session.UserID,
			Email:        session.Email,
			Role:         session.Role,
			AuthProvider: session.AuthProvider,
			Current:      session.ID == currentHash,
			IssuedAt:     session.IssuedAt,
			ExpiresAt:    session.ExpiresAt,
			LastSeenAt:   session.LastSeenAt,
			SourceIP:     session.SourceIP,
			UserAgent:    session.UserAgent,
		})
	}
	page := SessionCursorPage{Sessions: out}
	if hasMore && len(sessions) > 0 {
		page.NextCursor = encodeSessionCursor(sessions[len(sessions)-1])
	}
	return page, nil
}

func (s *Service) RevokeSessionByPublicID(ctx context.Context, actorUserID, actorRole, publicID string) (*SessionInventoryItem, error) {
	session, err := s.sessions.FindByPublicID(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("auth find session by public id: %w", err)
	}
	if session == nil || session.RevokedAt != nil {
		return nil, ErrInvalidSession
	}
	if actorRole != "admin" && session.UserID != actorUserID {
		return nil, ErrInvalidSession
	}
	reason := "session_revoke"
	if actorRole != "admin" {
		reason = "self_session_revoke"
	}
	if err := s.sessions.Revoke(ctx, session.ID, reason); err != nil {
		return nil, fmt.Errorf("auth revoke session by public id: %w", err)
	}
	return &SessionInventoryItem{
		SessionID:    session.PublicID,
		UserID:       session.UserID,
		Email:        session.Email,
		Role:         session.Role,
		AuthProvider: session.AuthProvider,
		Current:      false,
		IssuedAt:     session.IssuedAt,
		ExpiresAt:    session.ExpiresAt,
		LastSeenAt:   session.LastSeenAt,
		SourceIP:     session.SourceIP,
		UserAgent:    session.UserAgent,
	}, nil
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// generateToken creates a 32-byte cryptographically random token.
// Returns (rawToken hex string, SHA-256 hash hex string, error).
func generateToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := hex.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

// hashToken returns the hex-encoded SHA-256 hash of the raw token string.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HashRawSessionToken exposes the stable hash used as session storage identifier.
func HashRawSessionToken(raw string) string {
	return hashToken(raw)
}

func normalizeSessionAuthProvider(value string) string {
	if value == "oidc" {
		return "oidc"
	}
	return "password"
}

type sessionCursor struct {
	LastSeenAt string `json:"last_seen_at"`
	IssuedAt   string `json:"issued_at"`
	SessionID  string `json:"session_id"`
}

func encodeSessionCursor(session sessionrepo.Session) string {
	payload, _ := json.Marshal(sessionCursor{
		LastSeenAt: session.LastSeenAt.UTC().Format(time.RFC3339),
		IssuedAt:   session.IssuedAt.UTC().Format(time.RFC3339),
		SessionID:  session.PublicID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeSessionCursor(raw string) (time.Time, time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	var cursor sessionCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	if cursor.LastSeenAt == "" || cursor.IssuedAt == "" || cursor.SessionID == "" {
		return time.Time{}, time.Time{}, "", fmt.Errorf("cursor is incomplete")
	}
	lastSeenAt, err := time.Parse(time.RFC3339, cursor.LastSeenAt)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	issuedAt, err := time.Parse(time.RFC3339, cursor.IssuedAt)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	return lastSeenAt, issuedAt, cursor.SessionID, nil
}
