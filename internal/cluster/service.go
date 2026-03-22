package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zivego/wiregate/internal/persistence/clusterrepo"
)

type Config struct {
	Enabled           bool
	InstanceID        string
	LeaseName         string
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
}

type NodeStatus struct {
	InstanceID      string `json:"instance_id"`
	IsLeader        bool   `json:"is_leader"`
	LeaderID        string `json:"leader_id"`
	LastHeartbeatAt string `json:"last_heartbeat_at"`
	LeaseExpiresAt  string `json:"lease_expires_at,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}

type Status struct {
	Enabled          bool         `json:"enabled"`
	InstanceID       string       `json:"instance_id"`
	LeaseName        string       `json:"lease_name"`
	HeartbeatSeconds int          `json:"heartbeat_seconds"`
	LeaseSeconds     int          `json:"lease_seconds"`
	IsLeader         bool         `json:"is_leader"`
	LeaderID         string       `json:"leader_id"`
	LeaseExpiresAt   string       `json:"lease_expires_at,omitempty"`
	Nodes            []NodeStatus `json:"nodes"`
	LastUpdatedAt    string       `json:"last_updated_at,omitempty"`
}

type Service struct {
	repo *clusterrepo.Repo
	cfg  Config

	mu            sync.RWMutex
	isLeader      bool
	leaderID      string
	leaseExpires  time.Time
	lastUpdatedAt time.Time
}

func NewService(repo *clusterrepo.Repo, cfg Config) *Service {
	if cfg.LeaseName == "" {
		cfg.LeaseName = "wiregate-main"
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 15 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	return &Service{repo: repo, cfg: cfg}
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.repo == nil || !s.cfg.Enabled {
		return
	}
	go func() {
		s.tick(ctx)
		ticker := time.NewTicker(s.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Service) tick(ctx context.Context) {
	now := time.Now().UTC()
	acquired, lease, err := s.repo.TryAcquireLease(ctx, s.cfg.LeaseName, s.cfg.InstanceID, now, s.cfg.LeaseTTL)
	if err != nil {
		return
	}

	node := clusterrepo.Node{
		InstanceID:      s.cfg.InstanceID,
		IsLeader:        acquired,
		LeaderID:        lease.LeaderID,
		LastHeartbeatAt: now,
	}
	if !lease.LeaseExpiresAt.IsZero() {
		expires := lease.LeaseExpiresAt
		node.LeaseExpiresAt = &expires
	}
	if err := s.repo.UpsertNode(ctx, node); err != nil {
		return
	}

	s.mu.Lock()
	s.isLeader = acquired
	s.leaderID = lease.LeaderID
	s.leaseExpires = lease.LeaseExpiresAt
	s.lastUpdatedAt = now
	s.mu.Unlock()
}

func (s *Service) IsLeader() bool {
	if s == nil || !s.cfg.Enabled {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isLeader
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	if s == nil {
		return Status{}, fmt.Errorf("cluster service is not configured")
	}
	status := Status{
		Enabled:          s.cfg.Enabled,
		InstanceID:       s.cfg.InstanceID,
		LeaseName:        s.cfg.LeaseName,
		HeartbeatSeconds: int(s.cfg.HeartbeatInterval / time.Second),
		LeaseSeconds:     int(s.cfg.LeaseTTL / time.Second),
	}
	if !s.cfg.Enabled || s.repo == nil {
		return status, nil
	}

	s.mu.RLock()
	status.IsLeader = s.isLeader
	status.LeaderID = s.leaderID
	if !s.leaseExpires.IsZero() {
		status.LeaseExpiresAt = s.leaseExpires.UTC().Format(time.RFC3339)
	}
	if !s.lastUpdatedAt.IsZero() {
		status.LastUpdatedAt = s.lastUpdatedAt.UTC().Format(time.RFC3339)
	}
	s.mu.RUnlock()

	nodes, err := s.repo.ListNodes(ctx)
	if err != nil {
		return Status{}, err
	}
	out := make([]NodeStatus, 0, len(nodes))
	for _, node := range nodes {
		item := NodeStatus{
			InstanceID:      node.InstanceID,
			IsLeader:        node.IsLeader,
			LeaderID:        node.LeaderID,
			LastHeartbeatAt: node.LastHeartbeatAt.UTC().Format(time.RFC3339),
			UpdatedAt:       node.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if node.LeaseExpiresAt != nil {
			item.LeaseExpiresAt = node.LeaseExpiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	status.Nodes = out
	return status, nil
}
