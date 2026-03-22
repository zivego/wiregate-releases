package dns

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/enrollment"
	"github.com/zivego/wiregate/internal/persistence/dnsconfigrepo"
)

var (
	ErrInvalidConfig = errors.New("invalid dns config")
	domainPattern    = regexp.MustCompile(`^[a-zA-Z0-9.-]{1,253}$`)
)

type Config struct {
	Enabled       bool
	Servers       []string
	SearchDomains []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Service struct {
	repo *dnsconfigrepo.Repo
}

func NewService(repo *dnsconfigrepo.Repo) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetConfig(ctx context.Context) (Config, error) {
	if s == nil || s.repo == nil {
		return Config{}, nil
	}
	record, err := s.repo.Get(ctx)
	if err != nil {
		return Config{}, fmt.Errorf("dns get config: %w", err)
	}
	if record == nil {
		now := time.Now().UTC()
		record = &dnsconfigrepo.Config{
			ID:            "default",
			Enabled:       false,
			Servers:       []string{},
			SearchDomains: []string{},
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := s.repo.Upsert(ctx, *record); err != nil {
			return Config{}, fmt.Errorf("dns seed config: %w", err)
		}
	}
	return Config{
		Enabled:       record.Enabled,
		Servers:       slices.Clone(record.Servers),
		SearchDomains: slices.Clone(record.SearchDomains),
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}, nil
}

func (s *Service) UpdateConfig(ctx context.Context, config Config) (Config, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return Config{}, err
	}
	if s == nil || s.repo == nil {
		return normalized, nil
	}
	current, err := s.GetConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	record := dnsconfigrepo.Config{
		ID:            "default",
		Enabled:       normalized.Enabled,
		Servers:       slices.Clone(normalized.Servers),
		SearchDomains: slices.Clone(normalized.SearchDomains),
		CreatedAt:     current.CreatedAt,
		UpdatedAt:     time.Now().UTC(),
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}
	if err := s.repo.Upsert(ctx, record); err != nil {
		return Config{}, fmt.Errorf("dns update config: %w", err)
	}
	return Config{
		Enabled:       record.Enabled,
		Servers:       slices.Clone(record.Servers),
		SearchDomains: slices.Clone(record.SearchDomains),
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}, nil
}

func (s *Service) GetAgentDNSSettings(ctx context.Context) (enrollment.DNSSettings, error) {
	config, err := s.GetConfig(ctx)
	if err != nil {
		return enrollment.DNSSettings{}, err
	}
	return enrollment.DNSSettings{
		Enabled:       config.Enabled,
		Servers:       slices.Clone(config.Servers),
		SearchDomains: slices.Clone(config.SearchDomains),
	}, nil
}

func normalizeConfig(config Config) (Config, error) {
	normalized := Config{
		Enabled:       config.Enabled,
		Servers:       dedupeAndTrim(config.Servers),
		SearchDomains: dedupeAndTrim(config.SearchDomains),
	}
	if normalized.Enabled && len(normalized.Servers) == 0 {
		return Config{}, fmt.Errorf("%w: at least one DNS server is required when DNS is enabled", ErrInvalidConfig)
	}
	for _, server := range normalized.Servers {
		addr, err := netip.ParseAddr(server)
		if err != nil || !addr.IsValid() {
			return Config{}, fmt.Errorf("%w: invalid DNS server %q", ErrInvalidConfig, server)
		}
	}
	for _, domain := range normalized.SearchDomains {
		if !isValidDomain(domain) {
			return Config{}, fmt.Errorf("%w: invalid search domain %q", ErrInvalidConfig, domain)
		}
	}
	return normalized, nil
}

func dedupeAndTrim(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func isValidDomain(value string) bool {
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if value == "" || strings.Contains(value, "..") || !domainPattern.MatchString(value) {
		return false
	}
	return !strings.HasPrefix(value, "-") && !strings.HasSuffix(value, "-")
}
