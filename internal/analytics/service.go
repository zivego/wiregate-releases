package analytics

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/persistence/analyticsrepo"
)

var ErrInvalidRange = errors.New("invalid analytics range")
var ErrInvalidBucket = errors.New("invalid analytics bucket")

const (
	rollupMetricAuditTotal        = "audit.total"
	rollupMetricAuditAuthSecurity = "audit.auth_security"
)

type DashboardSeriesPoint struct {
	BucketStart time.Time
	Count       int
}

type EnrollmentFunnel struct {
	Issued           int
	Used             int
	RevokedOrExpired int
}

type PolicyCoverage struct {
	PoliciesTotal       int
	ActiveAssignments   int
	AgentsWithPolicy    int
	AgentsWithoutPolicy int
	TotalAgents         int
	CoveragePercent     float64
}

type DashboardHealthCards struct {
	TotalAgents        int
	RecentlySeenAgents int
	AppliedAgents      int
	DriftedAgents      int
	FailedAgents       int
	PendingReconcile   int
}

type FailingAgent struct {
	AgentID           string
	Hostname          string
	Platform          string
	Status            string
	LastApplyStatus   string
	LastApplyError    string
	RuntimeDriftState string
	FailureScore      int
	FailureCategories []string
}

type DashboardAnalytics struct {
	Range             string
	Bucket            string
	GeneratedAt       time.Time
	AuthSecurityTrend []DashboardSeriesPoint
	EnrollmentFunnel  EnrollmentFunnel
	PolicyCoverage    PolicyCoverage
	TopFailingAgents  []FailingAgent
	HealthCards       DashboardHealthCards
	LogDelivery       any
}

type ActionDistributionItem struct {
	Category string
	Count    int
}

type AuditHeatmapCell struct {
	Weekday int
	Hour    int
	Count   int
}

type AuditAnalytics struct {
	Range              string
	Bucket             string
	GeneratedAt        time.Time
	EventTrend         []DashboardSeriesPoint
	ActionDistribution []ActionDistributionItem
	ActivityHeatmap    []AuditHeatmapCell
	ExportIssues       []map[string]any
}

type Service struct {
	repo *analyticsrepo.Repo
	now  func() time.Time
}

func NewService(repo *analyticsrepo.Repo, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

func (s *Service) Dashboard(ctx context.Context, rangeKey string) (DashboardAnalytics, error) {
	spec, err := resolveRange(s.now().UTC(), rangeKey)
	if err != nil {
		return DashboardAnalytics{}, err
	}

	authSecurityTrend, err := s.loadOrBuildAuditTrend(ctx, spec, rollupMetricAuditAuthSecurity, filterAuthSecurityEvents)
	if err != nil {
		return DashboardAnalytics{}, err
	}

	tokens, err := s.repo.LoadEnrollmentTokens(ctx)
	if err != nil {
		return DashboardAnalytics{}, fmt.Errorf("analytics dashboard enrollment tokens: %w", err)
	}
	agents, err := s.repo.LoadAgents(ctx)
	if err != nil {
		return DashboardAnalytics{}, fmt.Errorf("analytics dashboard agents: %w", err)
	}
	runtimeStates, err := s.repo.LoadRuntimeSyncStates(ctx)
	if err != nil {
		return DashboardAnalytics{}, fmt.Errorf("analytics dashboard runtime sync: %w", err)
	}
	assignments, err := s.repo.LoadPolicyAssignments(ctx)
	if err != nil {
		return DashboardAnalytics{}, fmt.Errorf("analytics dashboard assignments: %w", err)
	}
	policiesTotal, err := s.repo.CountPolicies(ctx)
	if err != nil {
		return DashboardAnalytics{}, fmt.Errorf("analytics dashboard policies: %w", err)
	}

	now := s.now().UTC()
	return DashboardAnalytics{
		Range:             spec.Key,
		Bucket:            spec.Bucket,
		GeneratedAt:       now,
		AuthSecurityTrend: authSecurityTrend,
		EnrollmentFunnel:  buildEnrollmentFunnel(tokens, spec.Start, now),
		PolicyCoverage:    buildPolicyCoverage(policiesTotal, assignments, agents),
		TopFailingAgents:  buildTopFailingAgents(agents, runtimeStates),
		HealthCards:       buildDashboardHealthCards(agents, runtimeStates, now),
		LogDelivery:       nil,
	}, nil
}

func (s *Service) Audit(ctx context.Context, rangeKey, bucket string) (AuditAnalytics, error) {
	spec, err := resolveAuditRange(s.now().UTC(), rangeKey, bucket)
	if err != nil {
		return AuditAnalytics{}, err
	}
	eventTrend, err := s.loadOrBuildAuditTrend(ctx, spec, rollupMetricAuditTotal, nil)
	if err != nil {
		return AuditAnalytics{}, err
	}
	auditEvents, err := s.repo.LoadAuditEventsSince(ctx, spec.Start)
	if err != nil {
		return AuditAnalytics{}, fmt.Errorf("analytics audit events: %w", err)
	}

	return AuditAnalytics{
		Range:              spec.Key,
		Bucket:             spec.Bucket,
		GeneratedAt:        s.now().UTC(),
		EventTrend:         eventTrend,
		ActionDistribution: buildActionDistribution(auditEvents),
		ActivityHeatmap:    buildActivityHeatmap(auditEvents),
		ExportIssues:       []map[string]any{},
	}, nil
}

func (s *Service) loadOrBuildAuditTrend(ctx context.Context, spec rangeSpec, metric string, filter func([]analyticsrepo.AuditEvent) []analyticsrepo.AuditEvent) ([]DashboardSeriesPoint, error) {
	end := spec.Points[len(spec.Points)-1].Add(spec.Step)
	expectedCount, err := s.repo.CountAuditEvents(ctx, spec.Start, end, metric == rollupMetricAuditAuthSecurity)
	if err != nil {
		return nil, fmt.Errorf("analytics count audit events: %w", err)
	}

	rollups, err := s.repo.LoadRollupSeries(ctx, metric, spec.Bucket, spec.Start, end)
	if err != nil {
		return nil, fmt.Errorf("analytics load rollups: %w", err)
	}
	if rollupSeriesFresh(spec, rollups, expectedCount) {
		return mapRollupSeries(spec, rollups), nil
	}

	auditEvents, err := s.repo.LoadAuditEventsSince(ctx, spec.Start)
	if err != nil {
		return nil, fmt.Errorf("analytics load audit events for rollups: %w", err)
	}
	if filter != nil {
		auditEvents = filter(auditEvents)
	}
	trend := aggregateTrend(spec, auditEvents)
	if err := s.repo.ReplaceRollupSeries(ctx, metric, spec.Bucket, spec.Start, end, s.now().UTC(), toRollupSeries(metric, spec.Bucket, trend)); err != nil {
		return nil, fmt.Errorf("analytics store rollups: %w", err)
	}
	return trend, nil
}

type rangeSpec struct {
	Key    string
	Bucket string
	Start  time.Time
	Points []time.Time
	Step   time.Duration
}

func resolveRange(now time.Time, key string) (rangeSpec, error) {
	switch strings.TrimSpace(key) {
	case "24h", "":
		end := now.Truncate(time.Hour)
		start := end.Add(-23 * time.Hour)
		return buildRangeSpec("24h", "hour", start, 24, time.Hour), nil
	case "7d":
		end := truncateDay(now)
		start := end.AddDate(0, 0, -6)
		return buildRangeSpec("7d", "day", start, 7, 24*time.Hour), nil
	case "30d":
		end := truncateDay(now)
		start := end.AddDate(0, 0, -29)
		return buildRangeSpec("30d", "day", start, 30, 24*time.Hour), nil
	default:
		return rangeSpec{}, ErrInvalidRange
	}
}

func resolveAuditRange(now time.Time, key, bucket string) (rangeSpec, error) {
	spec, err := resolveRange(now, key)
	if err != nil {
		return rangeSpec{}, err
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return spec, nil
	}
	if bucket == "day" {
		start := truncateDay(spec.Start)
		end := truncateDay(now)
		count := int(end.Sub(start)/(24*time.Hour)) + 1
		return buildRangeSpec(spec.Key, "day", start, count, 24*time.Hour), nil
	}
	if bucket == "hour" && spec.Key == "24h" {
		return spec, nil
	}
	return rangeSpec{}, ErrInvalidBucket
}

func buildRangeSpec(key, bucket string, start time.Time, count int, step time.Duration) rangeSpec {
	points := make([]time.Time, 0, count)
	current := start
	for i := 0; i < count; i++ {
		points = append(points, current)
		current = current.Add(step)
	}
	return rangeSpec{
		Key:    key,
		Bucket: bucket,
		Start:  start,
		Points: points,
		Step:   step,
	}
}

func truncateDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func filterAuthSecurityEvents(events []analyticsrepo.AuditEvent) []analyticsrepo.AuditEvent {
	out := make([]analyticsrepo.AuditEvent, 0, len(events))
	for _, event := range events {
		if strings.HasPrefix(event.Action, "auth.") || strings.HasPrefix(event.Action, "security.") || strings.HasPrefix(event.Action, "session.") {
			out = append(out, event)
		}
	}
	return out
}

func aggregateTrend(spec rangeSpec, events []analyticsrepo.AuditEvent) []DashboardSeriesPoint {
	indexByBucket := make(map[time.Time]int, len(spec.Points))
	points := make([]DashboardSeriesPoint, 0, len(spec.Points))
	for idx, bucketStart := range spec.Points {
		indexByBucket[bucketStart] = idx
		points = append(points, DashboardSeriesPoint{BucketStart: bucketStart})
	}
	for _, event := range events {
		bucketStart := bucketFloor(event.CreatedAt.UTC(), spec.Bucket)
		if idx, ok := indexByBucket[bucketStart]; ok {
			points[idx].Count++
		}
	}
	return points
}

func rollupSeriesFresh(spec rangeSpec, rollups []analyticsrepo.RollupPoint, expectedCount int) bool {
	if len(rollups) != len(spec.Points) {
		return false
	}
	indexByBucket := make(map[time.Time]analyticsrepo.RollupPoint, len(rollups))
	total := 0
	for _, point := range rollups {
		indexByBucket[point.BucketStart.UTC()] = point
		total += point.Value
	}
	if total != expectedCount {
		return false
	}
	for _, bucketStart := range spec.Points {
		if _, ok := indexByBucket[bucketStart.UTC()]; !ok {
			return false
		}
	}
	return true
}

func mapRollupSeries(spec rangeSpec, rollups []analyticsrepo.RollupPoint) []DashboardSeriesPoint {
	values := make(map[time.Time]int, len(rollups))
	for _, point := range rollups {
		values[point.BucketStart.UTC()] = point.Value
	}
	out := make([]DashboardSeriesPoint, 0, len(spec.Points))
	for _, bucketStart := range spec.Points {
		out = append(out, DashboardSeriesPoint{
			BucketStart: bucketStart,
			Count:       values[bucketStart.UTC()],
		})
	}
	return out
}

func toRollupSeries(metric, bucket string, points []DashboardSeriesPoint) []analyticsrepo.RollupPoint {
	out := make([]analyticsrepo.RollupPoint, 0, len(points))
	for _, point := range points {
		out = append(out, analyticsrepo.RollupPoint{
			Metric:      metric,
			Bucket:      bucket,
			BucketStart: point.BucketStart.UTC(),
			Value:       point.Count,
		})
	}
	return out
}

func bucketFloor(value time.Time, bucket string) time.Time {
	if bucket == "day" {
		return truncateDay(value.UTC())
	}
	return value.UTC().Truncate(time.Hour)
}

func buildEnrollmentFunnel(tokens []analyticsrepo.EnrollmentToken, start, now time.Time) EnrollmentFunnel {
	var funnel EnrollmentFunnel
	for _, token := range tokens {
		if !token.CreatedAt.Before(start) {
			funnel.Issued++
		}
		if token.UsedAt != nil && !token.UsedAt.Before(start) {
			funnel.Used++
		}
		if token.RevokedAt != nil && !token.RevokedAt.Before(start) {
			funnel.RevokedOrExpired++
			continue
		}
		if token.RevokedAt == nil && token.UsedAt == nil && !token.ExpiresAt.Before(start) && !token.ExpiresAt.After(now) {
			funnel.RevokedOrExpired++
		}
	}
	return funnel
}

func buildPolicyCoverage(policiesTotal int, assignments []analyticsrepo.PolicyAssignment, agents []analyticsrepo.Agent) PolicyCoverage {
	activeAssignments := 0
	agentsWithPolicy := map[string]struct{}{}
	for _, assignment := range assignments {
		if assignment.Status != "active" {
			continue
		}
		activeAssignments++
		agentsWithPolicy[assignment.AgentID] = struct{}{}
	}
	totalAgents := len(agents)
	coveragePercent := 0.0
	if totalAgents > 0 {
		coveragePercent = float64(len(agentsWithPolicy)) * 100 / float64(totalAgents)
	}
	return PolicyCoverage{
		PoliciesTotal:       policiesTotal,
		ActiveAssignments:   activeAssignments,
		AgentsWithPolicy:    len(agentsWithPolicy),
		AgentsWithoutPolicy: maxInt(totalAgents-len(agentsWithPolicy), 0),
		TotalAgents:         totalAgents,
		CoveragePercent:     coveragePercent,
	}
}

func buildTopFailingAgents(agents []analyticsrepo.Agent, runtimeStates []analyticsrepo.RuntimeSyncState) []FailingAgent {
	runtimeByPeerID := make(map[string]analyticsrepo.RuntimeSyncState, len(runtimeStates))
	for _, state := range runtimeStates {
		runtimeByPeerID[state.PeerID] = state
	}

	out := make([]FailingAgent, 0)
	for _, agent := range agents {
		runtimeState := runtimeByPeerID[agent.PeerID]
		score, categories := failureScore(agent, runtimeState)
		if score == 0 {
			continue
		}
		out = append(out, FailingAgent{
			AgentID:           agent.ID,
			Hostname:          agent.Hostname,
			Platform:          agent.Platform,
			Status:            agent.Status,
			LastApplyStatus:   agent.LastApplyStatus,
			LastApplyError:    agent.LastApplyError,
			RuntimeDriftState: runtimeState.DriftState,
			FailureScore:      score,
			FailureCategories: categories,
		})
	}
	slices.SortFunc(out, func(a, b FailingAgent) int {
		if a.FailureScore != b.FailureScore {
			return b.FailureScore - a.FailureScore
		}
		return strings.Compare(a.Hostname, b.Hostname)
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func failureScore(agent analyticsrepo.Agent, runtimeState analyticsrepo.RuntimeSyncState) (int, []string) {
	score := 0
	categorySet := map[string]struct{}{}
	switch agent.LastApplyStatus {
	case "apply_failed":
		score += 3
		categorySet["apply"] = struct{}{}
	case "drifted":
		score += 2
		categorySet["reconcile"] = struct{}{}
	}
	if strings.Contains(strings.ToLower(agent.LastApplyError), "security") {
		score += 2
		categorySet["security"] = struct{}{}
	}
	switch runtimeState.DriftState {
	case "reconcile_failed":
		score += 3
		categorySet["reconcile"] = struct{}{}
	case "config_outdated", "pending_reconcile", "missing_runtime", "pending_apply", "rotation_pending", "pending_disable", "drifted":
		score++
		categorySet["reconcile"] = struct{}{}
	case "apply_failed":
		score += 2
		categorySet["apply"] = struct{}{}
	}
	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	return score, categories
}

func buildDashboardHealthCards(agents []analyticsrepo.Agent, runtimeStates []analyticsrepo.RuntimeSyncState, now time.Time) DashboardHealthCards {
	cards := DashboardHealthCards{
		TotalAgents: len(agents),
	}
	for _, agent := range agents {
		if agent.LastSeenAt != nil && now.Sub(agent.LastSeenAt.UTC()) <= time.Hour {
			cards.RecentlySeenAgents++
		}
		switch agent.LastApplyStatus {
		case "applied":
			cards.AppliedAgents++
		case "drifted":
			cards.DriftedAgents++
		case "apply_failed":
			cards.FailedAgents++
		}
	}
	for _, state := range runtimeStates {
		switch state.DriftState {
		case "pending_reconcile", "pending_apply", "rotation_pending", "config_outdated", "missing_runtime", "pending_disable", "reconcile_failed":
			cards.PendingReconcile++
		}
	}
	return cards
}

func buildActionDistribution(events []analyticsrepo.AuditEvent) []ActionDistributionItem {
	counts := map[string]int{}
	for _, event := range events {
		category := event.Action
		if idx := strings.Index(category, "."); idx > 0 {
			category = category[:idx]
		}
		counts[category]++
	}
	out := make([]ActionDistributionItem, 0, len(counts))
	for category, count := range counts {
		out = append(out, ActionDistributionItem{Category: category, Count: count})
	}
	slices.SortFunc(out, func(a, b ActionDistributionItem) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Category, b.Category)
	})
	return out
}

func buildActivityHeatmap(events []analyticsrepo.AuditEvent) []AuditHeatmapCell {
	counts := map[[2]int]int{}
	for _, event := range events {
		weekday := int(event.CreatedAt.UTC().Weekday())
		hour := event.CreatedAt.UTC().Hour()
		counts[[2]int{weekday, hour}]++
	}
	out := make([]AuditHeatmapCell, 0, len(counts))
	for key, count := range counts {
		out = append(out, AuditHeatmapCell{Weekday: key[0], Hour: key[1], Count: count})
	}
	slices.SortFunc(out, func(a, b AuditHeatmapCell) int {
		if a.Weekday != b.Weekday {
			return a.Weekday - b.Weekday
		}
		return a.Hour - b.Hour
	})
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
