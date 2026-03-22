package security

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zivego/wiregate/internal/persistence/securityapprovalrepo"
	"github.com/zivego/wiregate/internal/persistence/securitypolicyrepo"
)

var (
	ErrApprovalConflict      = errors.New("approval already pending")
	ErrApprovalNotFound      = errors.New("approval not found")
	ErrApprovalStateConflict = errors.New("approval state conflict")
	ErrApprovalSelfDecision  = errors.New("approval requires a different admin")
	ErrInvalidApprovalCursor = errors.New("invalid approval cursor")
)

type Policy struct {
	RequiredAdminAMR    string
	RequiredAdminACR    string
	DualApprovalEnabled bool
}

type Approval struct {
	ID                string
	Action            string
	ResourceType      string
	ResourceID        string
	RequestPayload    map[string]any
	RequestedByUserID string
	ApprovedByUserID  string
	RejectedByUserID  string
	Status            string
	CreatedAt         time.Time
	DecidedAt         *time.Time
}

type ListApprovalsFilter struct {
	Status string
	Limit  int
	Cursor string
}

type ApprovalCursorPage struct {
	Approvals  []Approval
	NextCursor string
}

type Service struct {
	policies  *securitypolicyrepo.Repo
	approvals *securityapprovalrepo.Repo
	defaults  Policy
}

func NewService(policies *securitypolicyrepo.Repo, approvals *securityapprovalrepo.Repo, defaults Policy) *Service {
	return &Service{
		policies:  policies,
		approvals: approvals,
		defaults: Policy{
			RequiredAdminAMR:    strings.TrimSpace(defaults.RequiredAdminAMR),
			RequiredAdminACR:    strings.TrimSpace(defaults.RequiredAdminACR),
			DualApprovalEnabled: defaults.DualApprovalEnabled,
		},
	}
}

func (s *Service) GetPolicy(ctx context.Context) (Policy, error) {
	if s == nil || s.policies == nil {
		return s.defaults, nil
	}
	record, err := s.policies.Get(ctx)
	if err != nil {
		return Policy{}, fmt.Errorf("security get policy: %w", err)
	}
	if record == nil {
		now := time.Now().UTC()
		record = &securitypolicyrepo.Policy{
			ID:                  "default",
			RequiredAdminAMR:    s.defaults.RequiredAdminAMR,
			RequiredAdminACR:    s.defaults.RequiredAdminACR,
			DualApprovalEnabled: s.defaults.DualApprovalEnabled,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := s.policies.Upsert(ctx, *record); err != nil {
			return Policy{}, fmt.Errorf("security seed policy: %w", err)
		}
	}
	return Policy{
		RequiredAdminAMR:    record.RequiredAdminAMR,
		RequiredAdminACR:    record.RequiredAdminACR,
		DualApprovalEnabled: record.DualApprovalEnabled,
	}, nil
}

func (s *Service) UpdatePolicy(ctx context.Context, policy Policy) (Policy, error) {
	if s == nil || s.policies == nil {
		s.defaults = policy
		return policy, nil
	}
	current, err := s.GetPolicy(ctx)
	if err != nil {
		return Policy{}, err
	}
	record := securitypolicyrepo.Policy{
		ID:                  "default",
		RequiredAdminAMR:    strings.TrimSpace(policy.RequiredAdminAMR),
		RequiredAdminACR:    strings.TrimSpace(policy.RequiredAdminACR),
		DualApprovalEnabled: policy.DualApprovalEnabled,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
	if current.RequiredAdminAMR != "" || current.RequiredAdminACR != "" || current.DualApprovalEnabled != s.defaults.DualApprovalEnabled {
		if existing, err := s.policies.Get(ctx); err == nil && existing != nil {
			record.CreatedAt = existing.CreatedAt
		}
	}
	if err := s.policies.Upsert(ctx, record); err != nil {
		return Policy{}, fmt.Errorf("security update policy: %w", err)
	}
	return Policy{
		RequiredAdminAMR:    record.RequiredAdminAMR,
		RequiredAdminACR:    record.RequiredAdminACR,
		DualApprovalEnabled: record.DualApprovalEnabled,
	}, nil
}

func (s *Service) ValidateAdminOIDCClaims(ctx context.Context, amr []string, acr string) error {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return err
	}
	requiredACR := strings.TrimSpace(policy.RequiredAdminACR)
	if requiredACR != "" && !strings.EqualFold(requiredACR, strings.TrimSpace(acr)) {
		return errors.New("admin role requires stronger OIDC ACR")
	}
	requiredAMR := strings.TrimSpace(policy.RequiredAdminAMR)
	if requiredAMR == "" {
		return nil
	}
	for _, value := range amr {
		if strings.EqualFold(strings.TrimSpace(value), requiredAMR) {
			return nil
		}
	}
	return errors.New("admin role requires stronger OIDC AMR")
}

func (s *Service) CreateApproval(ctx context.Context, action, resourceType, resourceID, requestedByUserID string, payload map[string]any) (Approval, error) {
	if s == nil || s.approvals == nil {
		return Approval{}, fmt.Errorf("security create approval: repo is not configured")
	}
	existing, err := s.approvals.FindPendingByActionResource(ctx, action, resourceType, resourceID)
	if err != nil {
		return Approval{}, fmt.Errorf("security find pending approval: %w", err)
	}
	if existing != nil {
		return Approval{}, ErrApprovalConflict
	}
	payloadJSON := ""
	if len(payload) > 0 {
		raw, err := json.Marshal(payload)
		if err != nil {
			return Approval{}, fmt.Errorf("security marshal approval payload: %w", err)
		}
		payloadJSON = string(raw)
	}
	record := securityapprovalrepo.Approval{
		ID:                 uuid.NewString(),
		Action:             action,
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		RequestPayloadJSON: payloadJSON,
		RequestedByUserID:  requestedByUserID,
		Status:             "pending",
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.approvals.Insert(ctx, record); err != nil {
		return Approval{}, fmt.Errorf("security insert approval: %w", err)
	}
	return toDomainApproval(record)
}

func (s *Service) ListApprovals(ctx context.Context, filter ListApprovalsFilter) ([]Approval, error) {
	page, err := s.ListApprovalsPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Approvals, nil
}

func (s *Service) ListApprovalsPage(ctx context.Context, filter ListApprovalsFilter) (ApprovalCursorPage, error) {
	if s == nil || s.approvals == nil {
		return ApprovalCursorPage{}, nil
	}
	repoFilter := securityapprovalrepo.ListFilter{
		Status: strings.TrimSpace(filter.Status),
		Limit:  filter.Limit,
	}
	if strings.TrimSpace(filter.Cursor) != "" {
		cursorTime, cursorID, err := decodeApprovalCursor(filter.Cursor)
		if err != nil {
			return ApprovalCursorPage{}, ErrInvalidApprovalCursor
		}
		repoFilter.CursorTime = cursorTime
		repoFilter.CursorID = cursorID
	}
	records, hasMore, err := s.approvals.ListPage(ctx, repoFilter)
	if err != nil {
		return ApprovalCursorPage{}, fmt.Errorf("security list approvals: %w", err)
	}
	out := make([]Approval, 0, len(records))
	for _, record := range records {
		approval, err := toDomainApproval(record)
		if err != nil {
			return ApprovalCursorPage{}, err
		}
		out = append(out, approval)
	}
	page := ApprovalCursorPage{Approvals: out}
	if hasMore && len(records) > 0 {
		page.NextCursor = encodeApprovalCursor(records[len(records)-1])
	}
	return page, nil
}

func (s *Service) GetApproval(ctx context.Context, id string) (Approval, error) {
	if s == nil || s.approvals == nil {
		return Approval{}, ErrApprovalNotFound
	}
	record, err := s.approvals.FindByID(ctx, id)
	if err != nil {
		return Approval{}, fmt.Errorf("security get approval: %w", err)
	}
	if record == nil {
		return Approval{}, ErrApprovalNotFound
	}
	return toDomainApproval(*record)
}

func (s *Service) MarkApproved(ctx context.Context, approvalID, approvedByUserID string) (Approval, error) {
	approval, err := s.GetApproval(ctx, approvalID)
	if err != nil {
		return Approval{}, err
	}
	if approval.Status != "pending" {
		return Approval{}, ErrApprovalStateConflict
	}
	if approval.RequestedByUserID == approvedByUserID {
		return Approval{}, ErrApprovalSelfDecision
	}
	now := time.Now().UTC()
	if err := s.approvals.MarkApproved(ctx, approvalID, approvedByUserID, now); err != nil {
		return Approval{}, fmt.Errorf("security mark approved: %w", err)
	}
	approval.Status = "approved"
	approval.ApprovedByUserID = approvedByUserID
	approval.DecidedAt = &now
	return approval, nil
}

func (s *Service) MarkRejected(ctx context.Context, approvalID, rejectedByUserID string) (Approval, error) {
	approval, err := s.GetApproval(ctx, approvalID)
	if err != nil {
		return Approval{}, err
	}
	if approval.Status != "pending" {
		return Approval{}, ErrApprovalStateConflict
	}
	if approval.RequestedByUserID == rejectedByUserID {
		return Approval{}, ErrApprovalSelfDecision
	}
	now := time.Now().UTC()
	if err := s.approvals.MarkRejected(ctx, approvalID, rejectedByUserID, now); err != nil {
		return Approval{}, fmt.Errorf("security mark rejected: %w", err)
	}
	approval.Status = "rejected"
	approval.RejectedByUserID = rejectedByUserID
	approval.DecidedAt = &now
	return approval, nil
}

func toDomainApproval(record securityapprovalrepo.Approval) (Approval, error) {
	approval := Approval{
		ID:                record.ID,
		Action:            record.Action,
		ResourceType:      record.ResourceType,
		ResourceID:        record.ResourceID,
		RequestedByUserID: record.RequestedByUserID,
		ApprovedByUserID:  record.ApprovedByUserID,
		RejectedByUserID:  record.RejectedByUserID,
		Status:            record.Status,
		CreatedAt:         record.CreatedAt,
		DecidedAt:         record.DecidedAt,
	}
	if record.RequestPayloadJSON != "" {
		if err := json.Unmarshal([]byte(record.RequestPayloadJSON), &approval.RequestPayload); err != nil {
			return Approval{}, fmt.Errorf("security decode approval payload: %w", err)
		}
	}
	return approval, nil
}

type approvalCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeApprovalCursor(record securityapprovalrepo.Approval) string {
	payload, _ := json.Marshal(approvalCursor{
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
		ID:        record.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeApprovalCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", err
	}
	var cursor approvalCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return time.Time{}, "", err
	}
	if cursor.CreatedAt == "" || cursor.ID == "" {
		return time.Time{}, "", fmt.Errorf("cursor is incomplete")
	}
	createdAt, err := time.Parse(time.RFC3339, cursor.CreatedAt)
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, cursor.ID, nil
}
