package policy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/persistence/accesspolicyrepo"
	"github.com/zivego/wiregate/internal/persistence/agentrepo"
	"github.com/zivego/wiregate/internal/persistence/peerrepo"
	"github.com/zivego/wiregate/internal/persistence/policyassignmentrepo"
)

var (
	ErrInvalidPolicyRequest = errors.New("invalid policy request")
	ErrPolicyNotFound       = errors.New("policy not found")
	ErrAssignmentConflict   = errors.New("policy assignment conflict")
	ErrAssignmentNotFound   = errors.New("policy assignment not found")
	ErrAgentNotFound        = errors.New("agent not found")
	ErrInvalidPolicyCursor  = errors.New("invalid policy cursor")
)

const (
	TrafficModeStandard   = "standard"
	TrafficModeFullTunnel = "full_tunnel"
	TrafficModeInherit    = "inherit"
)

type Policy struct {
	ID               string
	Name             string
	Description      string
	Destinations     []string
	TrafficMode      string
	AssignedAgentIDs []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AgentTrafficMode struct {
	Effective string
	Override  string
}

type Assignment struct {
	ID             string
	AgentID        string
	AccessPolicyID string
	Status         string
	CreatedAt      time.Time
}

type ListPoliciesFilter struct {
	Limit  int
	Cursor string
}

type PolicyPage struct {
	Policies   []Policy
	NextCursor string
}

type CreatePolicyInput struct {
	Name         string
	Description  string
	Destinations []string
	TrafficMode  string
}

type UpdatePolicyInput struct {
	Name         string
	Description  string
	Destinations []string
	TrafficMode  string
}

type AssignPolicyInput struct {
	AgentID        string
	AccessPolicyID string
}

type SimulatePolicyInput struct {
	AgentID             string
	PolicyIDs           []string
	TrafficModeOverride string
}

type SimulationResult struct {
	AgentID              string
	PolicyIDs            []string
	EffectiveTrafficMode string
	AllowedIPs           []string
	Destinations         []string
	RouteProfile         string
}

type UnassignPolicyInput struct {
	AgentID        string
	AccessPolicyID string
}

// Service owns access policy rules and AllowedIPs rendering.
type Service struct {
	policies    *accesspolicyrepo.Repo
	assignments *policyassignmentrepo.Repo
	agents      *agentrepo.Repo
	peers       *peerrepo.Repo
}

func NewService(policies *accesspolicyrepo.Repo, assignments *policyassignmentrepo.Repo, agents *agentrepo.Repo, peers *peerrepo.Repo) *Service {
	return &Service{
		policies:    policies,
		assignments: assignments,
		agents:      agents,
		peers:       peers,
	}
}

func (s *Service) CreatePolicy(ctx context.Context, in CreatePolicyInput) (Policy, error) {
	if s == nil || s.policies == nil {
		return Policy{}, fmt.Errorf("policy create: repo is not configured")
	}

	destinations, err := normalizeDestinations(in.Destinations)
	if err != nil {
		return Policy{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Policy{}, fmt.Errorf("%w: name is required", ErrInvalidPolicyRequest)
	}
	trafficMode, err := normalizePolicyTrafficMode(in.TrafficMode)
	if err != nil {
		return Policy{}, err
	}

	id, err := newID()
	if err != nil {
		return Policy{}, fmt.Errorf("policy create id: %w", err)
	}
	now := time.Now().UTC()
	destinationsJSON, err := json.Marshal(destinations)
	if err != nil {
		return Policy{}, fmt.Errorf("policy create marshal destinations: %w", err)
	}

	record := accesspolicyrepo.Policy{
		ID:               id,
		Name:             name,
		Description:      strings.TrimSpace(in.Description),
		DestinationsJSON: string(destinationsJSON),
		TrafficMode:      trafficMode,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.policies.Insert(ctx, record); err != nil {
		return Policy{}, fmt.Errorf("policy create: %w", err)
	}
	return toDomainPolicy(record), nil
}

func (s *Service) ListPolicies(ctx context.Context) ([]Policy, error) {
	page, err := s.ListPoliciesPage(ctx, ListPoliciesFilter{})
	if err != nil {
		return nil, err
	}
	return page.Policies, nil
}

func (s *Service) ListPoliciesPage(ctx context.Context, filter ListPoliciesFilter) (PolicyPage, error) {
	if s == nil || s.policies == nil {
		return PolicyPage{}, nil
	}

	repoFilter := accesspolicyrepo.ListFilter{Limit: filter.Limit}
	if strings.TrimSpace(filter.Cursor) != "" {
		cursorTime, cursorID, err := decodePolicyCursor(filter.Cursor)
		if err != nil {
			return PolicyPage{}, ErrInvalidPolicyCursor
		}
		repoFilter.CursorTime = cursorTime
		repoFilter.CursorID = cursorID
	}

	records, hasMore, err := s.policies.ListPage(ctx, repoFilter)
	if err != nil {
		return PolicyPage{}, fmt.Errorf("policy list: %w", err)
	}

	policyIDs := make([]string, 0, len(records))
	for _, record := range records {
		policyIDs = append(policyIDs, record.ID)
	}
	assignmentsByPolicyID, err := s.listAssignedAgentIDsByPolicyIDs(ctx, policyIDs)
	if err != nil {
		return PolicyPage{}, err
	}

	out := make([]Policy, 0, len(records))
	for _, record := range records {
		policy := toDomainPolicy(record)
		policy.AssignedAgentIDs = assignmentsByPolicyID[policy.ID]
		out = append(out, policy)
	}
	page := PolicyPage{Policies: out}
	if hasMore && len(records) > 0 {
		page.NextCursor = encodePolicyCursor(records[len(records)-1])
	}
	return page, nil
}

func (s *Service) UpdatePolicy(ctx context.Context, id string, in UpdatePolicyInput) (Policy, error) {
	if s == nil || s.policies == nil {
		return Policy{}, fmt.Errorf("policy update: repo is not configured")
	}

	record, err := s.policies.FindByID(ctx, id)
	if err != nil {
		return Policy{}, fmt.Errorf("policy update: %w", err)
	}
	if record == nil {
		return Policy{}, ErrPolicyNotFound
	}

	destinations, err := normalizeDestinations(in.Destinations)
	if err != nil {
		return Policy{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Policy{}, fmt.Errorf("%w: name is required", ErrInvalidPolicyRequest)
	}
	trafficMode, err := normalizePolicyTrafficMode(in.TrafficMode)
	if err != nil {
		return Policy{}, err
	}
	destinationsJSON, err := json.Marshal(destinations)
	if err != nil {
		return Policy{}, fmt.Errorf("policy update marshal destinations: %w", err)
	}

	record.Name = name
	record.Description = strings.TrimSpace(in.Description)
	record.DestinationsJSON = string(destinationsJSON)
	record.TrafficMode = trafficMode
	record.UpdatedAt = time.Now().UTC()
	if err := s.policies.Update(ctx, *record); err != nil {
		return Policy{}, fmt.Errorf("policy update: %w", err)
	}

	assignments, err := s.assignments.ListByPolicyID(ctx, id)
	if err != nil {
		return Policy{}, fmt.Errorf("policy update assignments: %w", err)
	}
	for _, assignment := range assignments {
		if err := s.refreshPeerIntentForAgent(ctx, assignment.AgentID); err != nil {
			return Policy{}, err
		}
	}

	return toDomainPolicy(*record), nil
}

func (s *Service) ValidatePolicyIDs(ctx context.Context, policyIDs []string) error {
	if len(policyIDs) == 0 {
		return nil
	}
	normalized := uniqueSorted(policyIDs)
	records, err := s.policies.FindByIDs(ctx, normalized)
	if err != nil {
		return fmt.Errorf("policy validate ids: %w", err)
	}
	if len(records) != len(normalized) {
		return fmt.Errorf("%w: one or more access_policy_ids were not found", ErrInvalidPolicyRequest)
	}
	return nil
}

func (s *Service) AssignPolicy(ctx context.Context, in AssignPolicyInput) (Assignment, error) {
	if s == nil || s.policies == nil || s.assignments == nil || s.agents == nil {
		return Assignment{}, fmt.Errorf("policy assign: repos are not configured")
	}
	if err := s.ValidatePolicyIDs(ctx, []string{in.AccessPolicyID}); err != nil {
		if errors.Is(err, ErrInvalidPolicyRequest) {
			return Assignment{}, ErrPolicyNotFound
		}
		return Assignment{}, err
	}

	agent, err := s.agents.FindByID(ctx, in.AgentID)
	if err != nil {
		return Assignment{}, fmt.Errorf("policy assign find agent: %w", err)
	}
	if agent == nil {
		return Assignment{}, ErrAgentNotFound
	}

	existing, err := s.assignments.FindActiveByAgentPolicy(ctx, in.AgentID, in.AccessPolicyID)
	if err != nil {
		return Assignment{}, fmt.Errorf("policy assign find existing: %w", err)
	}
	if existing != nil {
		return Assignment{}, fmt.Errorf("%w: policy already assigned", ErrAssignmentConflict)
	}

	id, err := newID()
	if err != nil {
		return Assignment{}, fmt.Errorf("policy assign id: %w", err)
	}
	record := policyassignmentrepo.Assignment{
		ID:             id,
		AgentID:        in.AgentID,
		AccessPolicyID: in.AccessPolicyID,
		Status:         "active",
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.assignments.Insert(ctx, record); err != nil {
		return Assignment{}, fmt.Errorf("policy assign: %w", err)
	}
	if err := s.refreshPeerIntentForAgent(ctx, in.AgentID); err != nil {
		return Assignment{}, err
	}
	return toDomainAssignment(record), nil
}

func (s *Service) UnassignPolicy(ctx context.Context, in UnassignPolicyInput) (Assignment, error) {
	if s == nil || s.policies == nil || s.assignments == nil || s.agents == nil {
		return Assignment{}, fmt.Errorf("policy unassign: repos are not configured")
	}
	if err := s.ValidatePolicyIDs(ctx, []string{in.AccessPolicyID}); err != nil {
		if errors.Is(err, ErrInvalidPolicyRequest) {
			return Assignment{}, ErrPolicyNotFound
		}
		return Assignment{}, err
	}

	agent, err := s.agents.FindByID(ctx, in.AgentID)
	if err != nil {
		return Assignment{}, fmt.Errorf("policy unassign find agent: %w", err)
	}
	if agent == nil {
		return Assignment{}, ErrAgentNotFound
	}

	existing, err := s.assignments.FindActiveByAgentPolicy(ctx, in.AgentID, in.AccessPolicyID)
	if err != nil {
		return Assignment{}, fmt.Errorf("policy unassign find existing: %w", err)
	}
	if existing == nil {
		return Assignment{}, ErrAssignmentNotFound
	}

	deactivated, err := s.assignments.DeactivateActiveByAgentPolicy(ctx, in.AgentID, in.AccessPolicyID)
	if err != nil {
		return Assignment{}, fmt.Errorf("policy unassign deactivate: %w", err)
	}
	if !deactivated {
		return Assignment{}, ErrAssignmentNotFound
	}

	if err := s.refreshPeerIntentForAgent(ctx, in.AgentID); err != nil {
		return Assignment{}, err
	}

	unassigned := toDomainAssignment(*existing)
	unassigned.Status = "inactive"
	return unassigned, nil
}

func (s *Service) AssignPoliciesToAgent(ctx context.Context, agentID string, policyIDs []string) error {
	if len(policyIDs) == 0 {
		return nil
	}
	if err := s.ValidatePolicyIDs(ctx, policyIDs); err != nil {
		return err
	}

	agent, err := s.agents.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("policy assign policies find agent: %w", err)
	}
	if agent == nil {
		return ErrAgentNotFound
	}

	now := time.Now().UTC()
	for _, policyID := range uniqueSorted(policyIDs) {
		existing, err := s.assignments.FindActiveByAgentPolicy(ctx, agentID, policyID)
		if err != nil {
			return fmt.Errorf("policy assign policies find existing: %w", err)
		}
		if existing != nil {
			continue
		}
		id, err := newID()
		if err != nil {
			return fmt.Errorf("policy assign policies id: %w", err)
		}
		if err := s.assignments.Insert(ctx, policyassignmentrepo.Assignment{
			ID:             id,
			AgentID:        agentID,
			AccessPolicyID: policyID,
			Status:         "active",
			CreatedAt:      now,
		}); err != nil {
			return fmt.Errorf("policy assign policies: %w", err)
		}
	}

	return s.refreshPeerIntentForAgent(ctx, agentID)
}

func (s *Service) ListPoliciesForAgent(ctx context.Context, agentID string) ([]Policy, error) {
	assignments, err := s.assignments.ListByAgentID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("policy list for agent assignments: %w", err)
	}
	if len(assignments) == 0 {
		return nil, nil
	}

	policyIDs := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		policyIDs = append(policyIDs, assignment.AccessPolicyID)
	}
	records, err := s.policies.FindByIDs(ctx, uniqueSorted(policyIDs))
	if err != nil {
		return nil, fmt.Errorf("policy list for agent find policies: %w", err)
	}

	out := make([]Policy, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainPolicy(record))
	}
	slices.SortFunc(out, func(a, b Policy) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (s *Service) ListPolicyIDsForAgent(ctx context.Context, agentID string) ([]string, error) {
	policies, err := s.ListPoliciesForAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(policies))
	for _, policy := range policies {
		out = append(out, policy.ID)
	}
	return out, nil
}

func (s *Service) ListAssignedAgentIDs(ctx context.Context, policyID string) ([]string, error) {
	if s == nil || s.assignments == nil {
		return nil, nil
	}
	assignments, err := s.assignments.ListByPolicyID(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("policy list assigned agent ids: %w", err)
	}
	agentIDs := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		agentIDs = append(agentIDs, assignment.AgentID)
	}
	return uniqueSorted(agentIDs), nil
}

func (s *Service) listAssignedAgentIDsByPolicyIDs(ctx context.Context, policyIDs []string) (map[string][]string, error) {
	if s == nil || s.assignments == nil || len(policyIDs) == 0 {
		return map[string][]string{}, nil
	}
	assignments, err := s.assignments.ListByPolicyIDs(ctx, policyIDs)
	if err != nil {
		return nil, fmt.Errorf("policy list assigned agent ids by policies: %w", err)
	}
	out := make(map[string][]string, len(policyIDs))
	for _, policyID := range policyIDs {
		out[policyID] = []string{}
	}
	for _, assignment := range assignments {
		out[assignment.AccessPolicyID] = append(out[assignment.AccessPolicyID], assignment.AgentID)
	}
	for policyID, agentIDs := range out {
		out[policyID] = uniqueSorted(agentIDs)
	}
	return out, nil
}

func encodePolicyCursor(policy accesspolicyrepo.Policy) string {
	payload, _ := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}{
		CreatedAt: policy.CreatedAt.UTC().Format(time.RFC3339),
		ID:        policy.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodePolicyCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", err
	}
	var payload struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return time.Time{}, "", err
	}
	if payload.CreatedAt == "" || payload.ID == "" {
		return time.Time{}, "", fmt.Errorf("cursor is incomplete")
	}
	createdAt, err := time.Parse(time.RFC3339, payload.CreatedAt)
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, payload.ID, nil
}

func (s *Service) RenderAllowedIPsForAgent(ctx context.Context, agentID string) ([]string, error) {
	trafficMode, err := s.GetAgentTrafficMode(ctx, agentID)
	if err != nil {
		return nil, err
	}
	policies, err := s.ListPoliciesForAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return renderAllowedIPsForPolicies(trafficMode.Effective, policies), nil
}

func (s *Service) GetAgentTrafficMode(ctx context.Context, agentID string) (AgentTrafficMode, error) {
	if s == nil || s.agents == nil {
		return AgentTrafficMode{Effective: TrafficModeStandard}, nil
	}
	agent, err := s.agents.FindByID(ctx, agentID)
	if err != nil {
		return AgentTrafficMode{}, fmt.Errorf("policy get agent traffic mode find agent: %w", err)
	}
	if agent == nil {
		return AgentTrafficMode{}, ErrAgentNotFound
	}

	override, err := normalizeAgentTrafficModeOverride(agent.TrafficModeOverride)
	if err != nil {
		return AgentTrafficMode{}, fmt.Errorf("policy get agent traffic mode normalize override: %w", err)
	}
	if override != "" {
		return AgentTrafficMode{Effective: override, Override: override}, nil
	}

	policies, err := s.ListPoliciesForAgent(ctx, agentID)
	if err != nil {
		return AgentTrafficMode{}, err
	}
	return AgentTrafficMode{
		Effective: effectiveTrafficMode(override, policies),
		Override:  "",
	}, nil
}

func (s *Service) SimulatePolicyIntent(ctx context.Context, in SimulatePolicyInput) (SimulationResult, error) {
	if s == nil || s.policies == nil {
		return SimulationResult{}, fmt.Errorf("policy simulate: repo is not configured")
	}
	agentID := strings.TrimSpace(in.AgentID)
	requestedPolicyIDs := uniqueSorted(in.PolicyIDs)
	if agentID == "" && len(requestedPolicyIDs) == 0 {
		return SimulationResult{}, fmt.Errorf("%w: agent_id or at least one policy_id is required", ErrInvalidPolicyRequest)
	}

	override := ""
	if agentID != "" {
		if s.agents == nil {
			return SimulationResult{}, fmt.Errorf("policy simulate: agent repo is not configured")
		}
		agent, err := s.agents.FindByID(ctx, agentID)
		if err != nil {
			return SimulationResult{}, fmt.Errorf("policy simulate find agent: %w", err)
		}
		if agent == nil {
			return SimulationResult{}, ErrAgentNotFound
		}
		override, err = normalizeAgentTrafficModeOverride(agent.TrafficModeOverride)
		if err != nil {
			return SimulationResult{}, fmt.Errorf("policy simulate normalize agent override: %w", err)
		}
	}

	if strings.TrimSpace(in.TrafficModeOverride) != "" {
		normalizedOverride, err := normalizeAgentTrafficModeOverride(in.TrafficModeOverride)
		if err != nil {
			return SimulationResult{}, err
		}
		override = normalizedOverride
	}

	if len(requestedPolicyIDs) == 0 && agentID != "" {
		currentPolicyIDs, err := s.ListPolicyIDsForAgent(ctx, agentID)
		if err != nil {
			return SimulationResult{}, err
		}
		requestedPolicyIDs = currentPolicyIDs
	}

	policies, err := s.listPoliciesByIDs(ctx, requestedPolicyIDs)
	if err != nil {
		return SimulationResult{}, err
	}

	effectiveMode := effectiveTrafficMode(override, policies)
	destinations := unionDestinations(policies)
	return SimulationResult{
		AgentID:              agentID,
		PolicyIDs:            requestedPolicyIDs,
		EffectiveTrafficMode: effectiveMode,
		AllowedIPs:           renderAllowedIPsForPolicies(effectiveMode, policies),
		Destinations:         destinations,
		RouteProfile:         effectiveMode,
	}, nil
}

func (s *Service) SetAgentTrafficModeOverride(ctx context.Context, agentID, mode string) (AgentTrafficMode, error) {
	if s == nil || s.agents == nil {
		return AgentTrafficMode{}, fmt.Errorf("policy set agent traffic mode: repo is not configured")
	}
	agent, err := s.agents.FindByID(ctx, agentID)
	if err != nil {
		return AgentTrafficMode{}, fmt.Errorf("policy set agent traffic mode find agent: %w", err)
	}
	if agent == nil {
		return AgentTrafficMode{}, ErrAgentNotFound
	}

	override, err := normalizeAgentTrafficModeOverride(mode)
	if err != nil {
		return AgentTrafficMode{}, err
	}
	if err := s.agents.UpdateTrafficModeOverride(ctx, agentID, override); err != nil {
		return AgentTrafficMode{}, fmt.Errorf("policy set agent traffic mode update override: %w", err)
	}
	if err := s.refreshPeerIntentForAgent(ctx, agentID); err != nil {
		return AgentTrafficMode{}, err
	}
	return s.GetAgentTrafficMode(ctx, agentID)
}

func (s *Service) refreshPeerIntentForAgent(ctx context.Context, agentID string) error {
	if s.peers == nil {
		return nil
	}
	peer, err := s.peers.FindByAgentID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("policy refresh peer intent find peer: %w", err)
	}
	if peer == nil {
		return nil
	}
	allowedIPs, err := s.RenderAllowedIPsForAgent(ctx, agentID)
	if err != nil {
		return err
	}
	status := peer.Status
	if status == "" || status == "active" || status == "rotation_pending" {
		status = "planned"
	}
	if err := s.peers.UpdateIntent(ctx, peer.ID, allowedIPs, status); err != nil {
		return fmt.Errorf("policy refresh peer intent update peer: %w", err)
	}
	return nil
}

func (s *Service) listPoliciesByIDs(ctx context.Context, policyIDs []string) ([]Policy, error) {
	if len(policyIDs) == 0 {
		return nil, nil
	}
	if err := s.ValidatePolicyIDs(ctx, policyIDs); err != nil {
		if errors.Is(err, ErrInvalidPolicyRequest) {
			return nil, ErrPolicyNotFound
		}
		return nil, err
	}
	records, err := s.policies.FindByIDs(ctx, uniqueSorted(policyIDs))
	if err != nil {
		return nil, fmt.Errorf("policy list by ids: %w", err)
	}
	out := make([]Policy, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainPolicy(record))
	}
	slices.SortFunc(out, func(a, b Policy) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func renderAllowedIPsForPolicies(trafficMode string, policies []Policy) []string {
	if trafficMode == TrafficModeFullTunnel {
		return []string{"0.0.0.0/0"}
	}
	return unionDestinations(policies)
}

func unionDestinations(policies []Policy) []string {
	var union []string
	for _, item := range policies {
		union = append(union, item.Destinations...)
	}
	return uniqueSorted(union)
}

func toDomainPolicy(record accesspolicyrepo.Policy) Policy {
	var destinations []string
	_ = json.Unmarshal([]byte(record.DestinationsJSON), &destinations)
	trafficMode, _ := normalizePolicyTrafficMode(record.TrafficMode)
	return Policy{
		ID:               record.ID,
		Name:             record.Name,
		Description:      record.Description,
		Destinations:     destinations,
		TrafficMode:      trafficMode,
		AssignedAgentIDs: []string{},
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func toDomainAssignment(record policyassignmentrepo.Assignment) Assignment {
	return Assignment{
		ID:             record.ID,
		AgentID:        record.AgentID,
		AccessPolicyID: record.AccessPolicyID,
		Status:         record.Status,
		CreatedAt:      record.CreatedAt,
	}
}

func normalizeDestinations(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("%w: at least one destination is required", ErrInvalidPolicyRequest)
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		cidr := strings.TrimSpace(raw)
		if cidr == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid destination %q", ErrInvalidPolicyRequest, raw)
		}
		out = append(out, network.String())
	}
	out = uniqueSorted(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one destination is required", ErrInvalidPolicyRequest)
	}
	return out, nil
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func normalizePolicyTrafficMode(raw string) (string, error) {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return TrafficModeStandard, nil
	}
	switch mode {
	case TrafficModeStandard, TrafficModeFullTunnel:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: traffic_mode must be standard or full_tunnel", ErrInvalidPolicyRequest)
	}
}

func normalizeAgentTrafficModeOverride(raw string) (string, error) {
	mode := strings.TrimSpace(raw)
	if mode == "" || mode == TrafficModeInherit {
		return "", nil
	}
	switch mode {
	case TrafficModeStandard, TrafficModeFullTunnel:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: mode must be inherit, standard, or full_tunnel", ErrInvalidPolicyRequest)
	}
}

func effectiveTrafficMode(override string, policies []Policy) string {
	if override != "" {
		return override
	}
	for _, policy := range policies {
		if policy.TrafficMode == TrafficModeFullTunnel {
			return TrafficModeFullTunnel
		}
	}
	return TrafficModeStandard
}
