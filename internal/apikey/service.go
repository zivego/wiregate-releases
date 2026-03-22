package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zivego/wiregate/internal/auth"
	"github.com/zivego/wiregate/internal/persistence/serviceaccountrepo"
)

type ServiceAccount struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type APIKey struct {
	ID               string     `json:"id"`
	ServiceAccountID string     `json:"service_account_id"`
	Name             string     `json:"name"`
	KeyPrefix        string     `json:"key_prefix"`
	Status           string     `json:"status"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
}

type CreatedKey struct {
	Key APIKey
	Raw string
}

type Service struct {
	repo *serviceaccountrepo.Repo
}

func NewService(repo *serviceaccountrepo.Repo) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateServiceAccount(ctx context.Context, name, role string) (ServiceAccount, error) {
	if s == nil || s.repo == nil {
		return ServiceAccount{}, fmt.Errorf("apikey service is not configured")
	}
	name = strings.TrimSpace(name)
	role = strings.TrimSpace(role)
	if name == "" {
		return ServiceAccount{}, fmt.Errorf("service account name is required")
	}
	if !isValidRole(role) {
		return ServiceAccount{}, fmt.Errorf("role must be admin, operator, or readonly")
	}
	account := serviceaccountrepo.ServiceAccount{
		ID:     uuid.NewString(),
		Name:   name,
		Role:   role,
		Status: "active",
	}
	if err := s.repo.InsertAccount(ctx, account); err != nil {
		return ServiceAccount{}, err
	}
	created, err := s.repo.FindAccountByID(ctx, account.ID)
	if err != nil {
		return ServiceAccount{}, err
	}
	if created == nil {
		return ServiceAccount{}, fmt.Errorf("service account was not created")
	}
	return mapAccount(*created), nil
}

func (s *Service) ListServiceAccounts(ctx context.Context) ([]ServiceAccount, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceAccount, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, mapAccount(account))
	}
	return out, nil
}

func (s *Service) CreateAPIKey(ctx context.Context, accountID, name string, ttl time.Duration) (CreatedKey, error) {
	if s == nil || s.repo == nil {
		return CreatedKey{}, fmt.Errorf("apikey service is not configured")
	}
	account, err := s.repo.FindAccountByID(ctx, accountID)
	if err != nil {
		return CreatedKey{}, err
	}
	if account == nil {
		return CreatedKey{}, fmt.Errorf("service account not found")
	}
	if account.Status != "active" {
		return CreatedKey{}, fmt.Errorf("service account is not active")
	}
	if ttl < 0 {
		return CreatedKey{}, fmt.Errorf("ttl must be non-negative")
	}
	raw, prefix, hash, err := generateAPIKeyMaterial()
	if err != nil {
		return CreatedKey{}, err
	}
	var expiresAt *time.Time
	if ttl > 0 {
		t := time.Now().UTC().Add(ttl)
		expiresAt = &t
	}
	key := serviceaccountrepo.APIKey{
		ID:               uuid.NewString(),
		ServiceAccountID: account.ID,
		Name:             strings.TrimSpace(name),
		KeyPrefix:        prefix,
		TokenHash:        hash,
		Status:           "active",
		ExpiresAt:        expiresAt,
	}
	if err := s.repo.InsertKey(ctx, key); err != nil {
		return CreatedKey{}, err
	}
	keys, err := s.repo.ListKeysByAccount(ctx, account.ID)
	if err != nil {
		return CreatedKey{}, err
	}
	for _, item := range keys {
		if item.ID == key.ID {
			return CreatedKey{Key: mapKey(item), Raw: raw}, nil
		}
	}
	return CreatedKey{}, fmt.Errorf("api key was not created")
}

func (s *Service) ListAPIKeys(ctx context.Context, accountID string) ([]APIKey, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	keys, err := s.repo.ListKeysByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, mapKey(key))
	}
	return out, nil
}

func (s *Service) RevokeAPIKey(ctx context.Context, accountID, keyID string) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("apikey service is not configured")
	}
	return s.repo.RevokeKey(ctx, accountID, keyID, time.Now().UTC())
}

// AuthenticateAPIKey validates bearer API key and maps it to auth.Claims.
func (s *Service) AuthenticateAPIKey(ctx context.Context, rawToken string) (auth.Claims, bool, error) {
	if s == nil || s.repo == nil {
		return auth.Claims{}, false, nil
	}
	token := strings.TrimSpace(rawToken)
	if !strings.HasPrefix(token, "wgk_") {
		return auth.Claims{}, false, nil
	}
	found, err := s.repo.FindActiveKeyByHash(ctx, hashToken(token), time.Now().UTC())
	if err != nil {
		return auth.Claims{}, false, err
	}
	if found == nil {
		return auth.Claims{}, false, nil
	}
	now := time.Now().UTC()
	if err := s.repo.TouchKey(ctx, found.Key.ServiceAccountID, found.Key.ID, now); err != nil {
		return auth.Claims{}, false, err
	}
	claims := auth.Claims{
		UserID:             "service-account:" + found.Key.ServiceAccountID,
		Email:              found.AccountName,
		Role:               found.AccountRole,
		MustChangePassword: false,
		ThemePreference:    "light",
		SessionID:          found.Key.ID,
		RawSessionToken:    token,
		AuthProvider:       "api_key",
		LastSeenAt:         now,
		IssuedAt:           found.Key.CreatedAt,
	}
	if found.Key.ExpiresAt != nil {
		claims.ExpiresAt = *found.Key.ExpiresAt
	}
	return claims, true, nil
}

func mapAccount(account serviceaccountrepo.ServiceAccount) ServiceAccount {
	return ServiceAccount{
		ID:         account.ID,
		Name:       account.Name,
		Role:       account.Role,
		Status:     account.Status,
		CreatedAt:  account.CreatedAt,
		UpdatedAt:  account.UpdatedAt,
		LastUsedAt: account.LastUsedAt,
	}
}

func mapKey(key serviceaccountrepo.APIKey) APIKey {
	return APIKey{
		ID:               key.ID,
		ServiceAccountID: key.ServiceAccountID,
		Name:             key.Name,
		KeyPrefix:        key.KeyPrefix,
		Status:           key.Status,
		ExpiresAt:        key.ExpiresAt,
		CreatedAt:        key.CreatedAt,
		RevokedAt:        key.RevokedAt,
		LastUsedAt:       key.LastUsedAt,
	}
}

func isValidRole(role string) bool {
	switch role {
	case "admin", "operator", "readonly":
		return true
	default:
		return false
	}
}

func generateAPIKeyMaterial() (raw string, prefix string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	raw = "wgk_" + hex.EncodeToString(buf)
	prefix = raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	hash = hashToken(raw)
	return raw, prefix, hash, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
