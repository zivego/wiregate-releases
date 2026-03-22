package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/analytics"
)

type dashboardSeriesPointResponse struct {
	BucketStart string `json:"bucket_start"`
	Count       int    `json:"count"`
}

type policyCoverageResponse struct {
	PoliciesTotal       int     `json:"policies_total"`
	ActiveAssignments   int     `json:"active_assignments"`
	AgentsWithPolicy    int     `json:"agents_with_policy"`
	AgentsWithoutPolicy int     `json:"agents_without_policy"`
	TotalAgents         int     `json:"total_agents"`
	CoveragePercent     float64 `json:"coverage_percent"`
}

type dashboardHealthCardsResponse struct {
	TotalAgents        int `json:"total_agents"`
	RecentlySeenAgents int `json:"recently_seen_agents"`
	AppliedAgents      int `json:"applied_agents"`
	DriftedAgents      int `json:"drifted_agents"`
	FailedAgents       int `json:"failed_agents"`
	PendingReconcile   int `json:"pending_reconcile"`
}

type failingAgentResponse struct {
	AgentID           string   `json:"agent_id"`
	Hostname          string   `json:"hostname"`
	Platform          string   `json:"platform"`
	Status            string   `json:"status"`
	LastApplyStatus   string   `json:"last_apply_status,omitempty"`
	LastApplyError    string   `json:"last_apply_error,omitempty"`
	RuntimeDriftState string   `json:"runtime_drift_state,omitempty"`
	FailureScore      int      `json:"failure_score"`
	FailureCategories []string `json:"failure_categories,omitempty"`
}

type enrollmentFunnelResponse struct {
	Issued           int `json:"issued"`
	Used             int `json:"used"`
	RevokedOrExpired int `json:"revoked_or_expired"`
}

type dashboardAnalyticsResponse struct {
	Range             string                         `json:"range"`
	Bucket            string                         `json:"bucket"`
	GeneratedAt       string                         `json:"generated_at"`
	AuthSecurityTrend []dashboardSeriesPointResponse `json:"auth_security_trend"`
	EnrollmentFunnel  enrollmentFunnelResponse       `json:"enrollment_funnel"`
	PolicyCoverage    policyCoverageResponse         `json:"policy_coverage"`
	TopFailingAgents  []failingAgentResponse         `json:"top_failing_agents"`
	HealthCards       dashboardHealthCardsResponse   `json:"health_cards"`
	LogDelivery       any                            `json:"log_delivery"`
}

type actionDistributionResponse struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

type auditHeatmapCellResponse struct {
	Weekday int `json:"weekday"`
	Hour    int `json:"hour"`
	Count   int `json:"count"`
}

type auditAnalyticsResponse struct {
	Range              string                         `json:"range"`
	Bucket             string                         `json:"bucket"`
	GeneratedAt        string                         `json:"generated_at"`
	EventTrend         []dashboardSeriesPointResponse `json:"event_trend"`
	ActionDistribution []actionDistributionResponse   `json:"action_distribution"`
	ActivityHeatmap    []auditHeatmapCellResponse     `json:"activity_heatmap"`
	ExportIssues       []map[string]any               `json:"export_issues"`
}

func (r *Router) handleGetDashboardAnalytics(w http.ResponseWriter, req *http.Request) {
	if r.analyticsService == nil {
		writeError(w, http.StatusNotFound, "not_found", "analytics is unavailable")
		return
	}
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	result, err := r.analyticsService.Dashboard(req.Context(), strings.TrimSpace(req.URL.Query().Get("range")))
	if errors.Is(err, analytics.ErrInvalidRange) {
		writeError(w, http.StatusBadRequest, "validation_failed", "range must be one of 24h, 7d, or 30d")
		return
	}
	if err != nil {
		r.logger.Printf("dashboard analytics error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load dashboard analytics")
		return
	}

	resp := dashboardAnalyticsResponse{
		Range:             result.Range,
		Bucket:            result.Bucket,
		GeneratedAt:       result.GeneratedAt.Format(time.RFC3339Nano),
		AuthSecurityTrend: mapSeriesPoints(result.AuthSecurityTrend),
		EnrollmentFunnel: enrollmentFunnelResponse{
			Issued:           result.EnrollmentFunnel.Issued,
			Used:             result.EnrollmentFunnel.Used,
			RevokedOrExpired: result.EnrollmentFunnel.RevokedOrExpired,
		},
		PolicyCoverage: policyCoverageResponse{
			PoliciesTotal:       result.PolicyCoverage.PoliciesTotal,
			ActiveAssignments:   result.PolicyCoverage.ActiveAssignments,
			AgentsWithPolicy:    result.PolicyCoverage.AgentsWithPolicy,
			AgentsWithoutPolicy: result.PolicyCoverage.AgentsWithoutPolicy,
			TotalAgents:         result.PolicyCoverage.TotalAgents,
			CoveragePercent:     result.PolicyCoverage.CoveragePercent,
		},
		HealthCards: dashboardHealthCardsResponse{
			TotalAgents:        result.HealthCards.TotalAgents,
			RecentlySeenAgents: result.HealthCards.RecentlySeenAgents,
			AppliedAgents:      result.HealthCards.AppliedAgents,
			DriftedAgents:      result.HealthCards.DriftedAgents,
			FailedAgents:       result.HealthCards.FailedAgents,
			PendingReconcile:   result.HealthCards.PendingReconcile,
		},
		LogDelivery: result.LogDelivery,
	}
	resp.TopFailingAgents = make([]failingAgentResponse, 0, len(result.TopFailingAgents))
	for _, agent := range result.TopFailingAgents {
		resp.TopFailingAgents = append(resp.TopFailingAgents, failingAgentResponse{
			AgentID:           agent.AgentID,
			Hostname:          agent.Hostname,
			Platform:          agent.Platform,
			Status:            agent.Status,
			LastApplyStatus:   agent.LastApplyStatus,
			LastApplyError:    agent.LastApplyError,
			RuntimeDriftState: agent.RuntimeDriftState,
			FailureScore:      agent.FailureScore,
			FailureCategories: agent.FailureCategories,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) handleGetAuditAnalytics(w http.ResponseWriter, req *http.Request) {
	if r.analyticsService == nil {
		writeError(w, http.StatusNotFound, "not_found", "analytics is unavailable")
		return
	}
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	result, err := r.analyticsService.Audit(
		req.Context(),
		strings.TrimSpace(req.URL.Query().Get("range")),
		strings.TrimSpace(req.URL.Query().Get("bucket")),
	)
	if errors.Is(err, analytics.ErrInvalidRange) {
		writeError(w, http.StatusBadRequest, "validation_failed", "range must be one of 24h, 7d, or 30d")
		return
	}
	if errors.Is(err, analytics.ErrInvalidBucket) {
		writeError(w, http.StatusBadRequest, "validation_failed", "bucket must be a supported combination of hour or day for the selected range")
		return
	}
	if err != nil {
		r.logger.Printf("audit analytics error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load audit analytics")
		return
	}

	resp := auditAnalyticsResponse{
		Range:        result.Range,
		Bucket:       result.Bucket,
		GeneratedAt:  result.GeneratedAt.Format(time.RFC3339Nano),
		EventTrend:   mapSeriesPoints(result.EventTrend),
		ExportIssues: result.ExportIssues,
	}
	resp.ActionDistribution = make([]actionDistributionResponse, 0, len(result.ActionDistribution))
	for _, item := range result.ActionDistribution {
		resp.ActionDistribution = append(resp.ActionDistribution, actionDistributionResponse{
			Category: item.Category,
			Count:    item.Count,
		})
	}
	resp.ActivityHeatmap = make([]auditHeatmapCellResponse, 0, len(result.ActivityHeatmap))
	for _, cell := range result.ActivityHeatmap {
		resp.ActivityHeatmap = append(resp.ActivityHeatmap, auditHeatmapCellResponse{
			Weekday: cell.Weekday,
			Hour:    cell.Hour,
			Count:   cell.Count,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func mapSeriesPoints(points []analytics.DashboardSeriesPoint) []dashboardSeriesPointResponse {
	out := make([]dashboardSeriesPointResponse, 0, len(points))
	for _, point := range points {
		out = append(out, dashboardSeriesPointResponse{
			BucketStart: point.BucketStart.Format(time.RFC3339Nano),
			Count:       point.Count,
		})
	}
	return out
}
