package capacity

import (
	"context"
	"fmt"
	"time"

	wirelogging "github.com/zivego/wiregate/internal/logging"
	"github.com/zivego/wiregate/internal/persistence/capacityrepo"
)

type InventorySummary struct {
	Users               int `json:"users"`
	Agents              int `json:"agents"`
	Peers               int `json:"peers"`
	AccessPolicies      int `json:"access_policies"`
	PolicyAssignments   int `json:"policy_assignments"`
	EnrollmentTokens    int `json:"enrollment_tokens"`
	PendingApprovals    int `json:"pending_security_approvals"`
	RecentlySeenAgents  int `json:"recently_seen_agents"`
	FailedAgents        int `json:"failed_agents"`
	DriftedPeers        int `json:"drifted_peers"`
}

type SessionSummary struct {
	ActiveSessions     int `json:"active_sessions"`
	IdleTimeoutMinutes int `json:"idle_timeout_minutes"`
}

type AuditSummary struct {
	TotalEvents  int        `json:"total_events"`
	OldestEventAt *time.Time `json:"oldest_event_at,omitempty"`
	NewestEventAt *time.Time `json:"newest_event_at,omitempty"`
}

type LoggingSummary struct {
	QueueCapacity        int `json:"queue_capacity"`
	CurrentQueued        int `json:"current_queued"`
	SinksTotal           int `json:"sinks_total"`
	EnabledSinks         int `json:"enabled_sinks"`
	DegradedSinks        int `json:"degraded_sinks"`
	DroppedEvents        int `json:"dropped_events"`
	TotalDelivered       int `json:"total_delivered"`
	TotalFailed          int `json:"total_failed"`
	ConsecutiveFailures  int `json:"consecutive_failures"`
}

type StorageSummary struct {
	AnalyticsRollups int `json:"analytics_rollups"`
	LogDeadLetters   int `json:"log_dead_letters"`
}

type Snapshot struct {
	GeneratedAt    time.Time        `json:"generated_at"`
	DatabaseEngine string           `json:"database_engine"`
	Inventory      InventorySummary `json:"inventory"`
	Sessions       SessionSummary   `json:"sessions"`
	Audit          AuditSummary     `json:"audit"`
	Logging        LoggingSummary   `json:"logging"`
	Storage        StorageSummary   `json:"storage"`
}

type Service struct {
	repo        *capacityrepo.Repo
	logging     *wirelogging.Service
	now         func() time.Time
	idleTimeout time.Duration
}

func NewService(repo *capacityrepo.Repo, logging *wirelogging.Service, idleTimeout time.Duration, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Minute
	}
	return &Service{
		repo:        repo,
		logging:     logging,
		now:         now,
		idleTimeout: idleTimeout,
	}
}

func (s *Service) GetSnapshot(ctx context.Context) (Snapshot, error) {
	if s == nil || s.repo == nil {
		return Snapshot{}, fmt.Errorf("capacity service is not configured")
	}

	now := s.now().UTC()
	raw, err := s.repo.LoadSnapshot(ctx, now, now.Add(-s.idleTimeout), now.Add(-30*time.Minute))
	if err != nil {
		return Snapshot{}, fmt.Errorf("capacity snapshot: %w", err)
	}

	snapshot := Snapshot{
		GeneratedAt:    now,
		DatabaseEngine: s.repo.Engine(),
		Inventory: InventorySummary{
			Users:              raw.TotalUsers,
			Agents:             raw.TotalAgents,
			Peers:              raw.TotalPeers,
			AccessPolicies:     raw.TotalAccessPolicies,
			PolicyAssignments:  raw.TotalPolicyAssignments,
			EnrollmentTokens:   raw.TotalEnrollmentTokens,
			PendingApprovals:   raw.PendingSecurityApprovals,
			RecentlySeenAgents: raw.RecentlySeenAgents,
			FailedAgents:       raw.FailedAgents,
			DriftedPeers:       raw.DriftedPeers,
		},
		Sessions: SessionSummary{
			ActiveSessions:     raw.ActiveSessions,
			IdleTimeoutMinutes: int(s.idleTimeout / time.Minute),
		},
		Audit: AuditSummary{
			TotalEvents:   raw.TotalAuditEvents,
			OldestEventAt: raw.OldestAuditAt,
			NewestEventAt: raw.NewestAuditAt,
		},
		Storage: StorageSummary{
			AnalyticsRollups: raw.TotalAnalyticsRollups,
			LogDeadLetters:   raw.TotalLogDeadLetters,
		},
	}

	if s.logging != nil {
		sinks, err := s.logging.ListSinks(ctx)
		if err != nil {
			return Snapshot{}, fmt.Errorf("capacity logging sinks: %w", err)
		}
		status, err := s.logging.GetStatus(ctx)
		if err != nil {
			return Snapshot{}, fmt.Errorf("capacity logging status: %w", err)
		}
		snapshot.Logging = summarizeLogging(sinks, status)
	}

	return snapshot, nil
}

func summarizeLogging(sinks []wirelogging.Sink, status wirelogging.StatusSnapshot) LoggingSummary {
	summary := LoggingSummary{
		QueueCapacity: status.QueueCapacity,
		CurrentQueued: status.CurrentQueued,
		SinksTotal:    len(status.Sinks),
	}
	for _, sink := range sinks {
		if sink.Enabled {
			summary.EnabledSinks++
		}
	}
	for _, sink := range status.Sinks {
		if sink.QueueDepth > 0 || sink.LastError != "" || sink.ConsecutiveFailures > 0 || sink.DroppedEvents > 0 {
			summary.DegradedSinks++
		}
		summary.DroppedEvents += sink.DroppedEvents
		summary.TotalDelivered += sink.TotalDelivered
		summary.TotalFailed += sink.TotalFailed
		summary.ConsecutiveFailures += sink.ConsecutiveFailures
	}
	return summary
}
