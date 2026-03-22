package network

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/persistence/agentrepo"
	"github.com/zivego/wiregate/internal/persistence/peerrepo"
	"github.com/zivego/wiregate/internal/persistence/runtimesyncrepo"
	"github.com/zivego/wiregate/internal/policy"
)

var (
	ErrInvalidGatewayMode = errors.New("invalid gateway mode")
	ErrAgentNotFound      = errors.New("agent not found")
)

const (
	GatewayModeDisabled      = "disabled"
	GatewayModeSubnetAccess  = "subnet_access"
	GatewayModeEgressGateway = "egress_gateway"

	PathModeDirect = "direct"
	PathModeRelay  = "relay"

	RouteProfileStandard      = "standard"
	RouteProfileFullTunnel    = "full_tunnel"
	RouteProfileSubnetAccess  = "subnet_access"
	RouteProfileEgressGateway = "egress_gateway"

	GatewayAssignmentNotApplicable     = "not_applicable"
	GatewayAssignmentReady             = "ready"
	GatewayAssignmentPendingEnrollment = "pending_enrollment"
	GatewayAssignmentInactiveAgent     = "inactive_agent"
	GatewayAssignmentNeedsDestinations = "needs_destinations"
	GatewayAssignmentDrifted           = "drifted"

	RelayStatusUnavailable = "unavailable"
)

type DiagnosticsSnapshot struct {
	GeneratedAt    time.Time
	RelayAvailable bool
	RelayStatus    string
	Summary        DiagnosticsSummary
	Agents         []AgentDiagnostic
}

type DiagnosticsSummary struct {
	TotalAgents   int
	DirectAgents  int
	RelayAgents   int
	GatewayAgents int
	ConflictCount int
}

type AgentDiagnostic struct {
	AgentID                 string
	Hostname                string
	Platform                string
	AgentStatus             string
	PeerID                  string
	PeerStatus              string
	TrafficMode             string
	GatewayMode             string
	RouteProfile            string
	PathMode                string
	GatewayAssignmentStatus string
	DriftState              string
	AllowedDestinations     []string
	RouteConflicts          []string
	LastSeenAt              *time.Time
}

type Service struct {
	agents      *agentrepo.Repo
	peers       *peerrepo.Repo
	runtimeSync *runtimesyncrepo.Repo
	policies    policyReader
}

type policyReader interface {
	GetAgentTrafficMode(ctx context.Context, agentID string) (policy.AgentTrafficMode, error)
	ListPoliciesForAgent(ctx context.Context, agentID string) ([]policy.Policy, error)
}

func NewService(agents *agentrepo.Repo, peers *peerrepo.Repo, runtimeSync *runtimesyncrepo.Repo, policies policyReader) *Service {
	return &Service{
		agents:      agents,
		peers:       peers,
		runtimeSync: runtimeSync,
		policies:    policies,
	}
}

func (s *Service) SetAgentGatewayMode(ctx context.Context, agentID, mode string) (string, error) {
	if s == nil || s.agents == nil {
		return "", fmt.Errorf("network set gateway mode: repo is not configured")
	}
	agent, err := s.agents.FindByID(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("network set gateway mode find agent: %w", err)
	}
	if agent == nil {
		return "", ErrAgentNotFound
	}
	normalized, err := normalizeGatewayMode(mode)
	if err != nil {
		return "", err
	}
	if err := s.agents.UpdateGatewayMode(ctx, agentID, normalized); err != nil {
		return "", fmt.Errorf("network set gateway mode update agent: %w", err)
	}
	return normalized, nil
}

func (s *Service) GetDiagnosticsSnapshot(ctx context.Context) (DiagnosticsSnapshot, error) {
	if s == nil || s.agents == nil {
		return DiagnosticsSnapshot{
			GeneratedAt: time.Now().UTC(),
			RelayStatus: RelayStatusUnavailable,
		}, nil
	}

	agents, err := s.agents.List(ctx, agentrepo.ListFilter{})
	if err != nil {
		return DiagnosticsSnapshot{}, fmt.Errorf("network diagnostics list agents: %w", err)
	}
	peersByAgent, err := s.listPeersByAgent(ctx)
	if err != nil {
		return DiagnosticsSnapshot{}, err
	}
	runtimeByPeer, err := s.listRuntimeByPeer(ctx)
	if err != nil {
		return DiagnosticsSnapshot{}, err
	}

	snapshot := DiagnosticsSnapshot{
		GeneratedAt:    time.Now().UTC(),
		RelayAvailable: false,
		RelayStatus:    RelayStatusUnavailable,
		Agents:         make([]AgentDiagnostic, 0, len(agents)),
	}
	for _, agent := range agents {
		diagnostic, err := s.buildAgentDiagnostic(ctx, agent, peersByAgent[agent.ID], runtimeByPeer)
		if err != nil {
			return DiagnosticsSnapshot{}, err
		}
		snapshot.Agents = append(snapshot.Agents, diagnostic)
		snapshot.Summary.TotalAgents++
		if diagnostic.PathMode == PathModeRelay {
			snapshot.Summary.RelayAgents++
		} else {
			snapshot.Summary.DirectAgents++
		}
		if diagnostic.GatewayMode != GatewayModeDisabled {
			snapshot.Summary.GatewayAgents++
		}
		snapshot.Summary.ConflictCount += len(diagnostic.RouteConflicts)
	}

	slices.SortFunc(snapshot.Agents, func(a, b AgentDiagnostic) int {
		return strings.Compare(strings.ToLower(a.Hostname), strings.ToLower(b.Hostname))
	})
	return snapshot, nil
}

func (s *Service) buildAgentDiagnostic(ctx context.Context, agent agentrepo.Agent, peer *peerrepo.Peer, runtimeByPeer map[string]runtimesyncrepo.State) (AgentDiagnostic, error) {
	trafficMode := policy.AgentTrafficMode{Effective: policy.TrafficModeStandard}
	if s.policies != nil {
		resolved, err := s.policies.GetAgentTrafficMode(ctx, agent.ID)
		if err != nil && !errors.Is(err, policy.ErrAgentNotFound) {
			return AgentDiagnostic{}, fmt.Errorf("network diagnostics traffic mode: %w", err)
		}
		if resolved.Effective != "" {
			trafficMode = resolved
		}
	}

	var policiesForAgent []policy.Policy
	if s.policies != nil {
		resolved, err := s.policies.ListPoliciesForAgent(ctx, agent.ID)
		if err != nil {
			return AgentDiagnostic{}, fmt.Errorf("network diagnostics policies for agent: %w", err)
		}
		policiesForAgent = resolved
	}

	gatewayMode, err := normalizeGatewayMode(agent.GatewayMode)
	if err != nil {
		gatewayMode = GatewayModeDisabled
	}
	allowedDestinations := collectAllowedDestinations(policiesForAgent)
	driftState := ""
	if peer != nil {
		if runtimeState, ok := runtimeByPeer[peer.ID]; ok {
			driftState = strings.TrimSpace(runtimeState.DriftState)
		}
	}

	diagnostic := AgentDiagnostic{
		AgentID:                 agent.ID,
		Hostname:                agent.Hostname,
		Platform:                agent.Platform,
		AgentStatus:             agent.Status,
		TrafficMode:             trafficMode.Effective,
		GatewayMode:             gatewayMode,
		RouteProfile:            deriveRouteProfile(trafficMode.Effective, gatewayMode),
		PathMode:                PathModeDirect,
		GatewayAssignmentStatus: deriveGatewayAssignmentStatus(agent, peer, gatewayMode, allowedDestinations, driftState),
		DriftState:              driftState,
		AllowedDestinations:     allowedDestinations,
		RouteConflicts:          buildRouteConflicts(agent, peer, gatewayMode, driftState, allowedDestinations),
		LastSeenAt:              agent.LastSeenAt,
	}
	if diagnostic.TrafficMode == "" {
		diagnostic.TrafficMode = policy.TrafficModeStandard
	}
	if peer != nil {
		diagnostic.PeerID = peer.ID
		diagnostic.PeerStatus = peer.Status
	}
	return diagnostic, nil
}

func (s *Service) listPeersByAgent(ctx context.Context) (map[string]*peerrepo.Peer, error) {
	if s.peers == nil {
		return map[string]*peerrepo.Peer{}, nil
	}
	peers, err := s.peers.List(ctx, peerrepo.ListFilter{})
	if err != nil {
		return nil, fmt.Errorf("network diagnostics list peers: %w", err)
	}
	out := make(map[string]*peerrepo.Peer, len(peers))
	for idx := range peers {
		peer := peers[idx]
		out[peer.AgentID] = &peer
	}
	return out, nil
}

func (s *Service) listRuntimeByPeer(ctx context.Context) (map[string]runtimesyncrepo.State, error) {
	if s.runtimeSync == nil {
		return map[string]runtimesyncrepo.State{}, nil
	}
	states, err := s.runtimeSync.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("network diagnostics list runtime sync: %w", err)
	}
	out := make(map[string]runtimesyncrepo.State, len(states))
	for _, state := range states {
		if strings.TrimSpace(state.PeerID) == "" {
			continue
		}
		out[state.PeerID] = state
	}
	return out, nil
}

func collectAllowedDestinations(policies []policy.Policy) []string {
	var out []string
	for _, policy := range policies {
		out = append(out, policy.Destinations...)
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func deriveRouteProfile(trafficMode, gatewayMode string) string {
	switch gatewayMode {
	case GatewayModeSubnetAccess:
		return RouteProfileSubnetAccess
	case GatewayModeEgressGateway:
		return RouteProfileEgressGateway
	}
	if strings.TrimSpace(trafficMode) == policy.TrafficModeFullTunnel {
		return RouteProfileFullTunnel
	}
	return RouteProfileStandard
}

func deriveGatewayAssignmentStatus(agent agentrepo.Agent, peer *peerrepo.Peer, gatewayMode string, allowedDestinations []string, driftState string) string {
	if gatewayMode == GatewayModeDisabled {
		return GatewayAssignmentNotApplicable
	}
	if agent.Status == "disabled" || agent.Status == "revoked" {
		return GatewayAssignmentInactiveAgent
	}
	if peer == nil {
		return GatewayAssignmentPendingEnrollment
	}
	if gatewayMode == GatewayModeSubnetAccess && len(allowedDestinations) == 0 {
		return GatewayAssignmentNeedsDestinations
	}
	if driftState == "drifted" {
		return GatewayAssignmentDrifted
	}
	return GatewayAssignmentReady
}

func buildRouteConflicts(agent agentrepo.Agent, peer *peerrepo.Peer, gatewayMode, driftState string, allowedDestinations []string) []string {
	if gatewayMode == GatewayModeDisabled {
		return nil
	}
	var conflicts []string
	if agent.Status == "disabled" || agent.Status == "revoked" {
		conflicts = append(conflicts, "gateway agent is not active")
	}
	if peer == nil {
		conflicts = append(conflicts, "gateway agent has no enrolled peer")
	}
	if gatewayMode == GatewayModeSubnetAccess && len(allowedDestinations) == 0 {
		conflicts = append(conflicts, "subnet gateway has no routed destinations")
	}
	if driftState == "drifted" {
		conflicts = append(conflicts, "gateway intent is drifted")
	}
	return conflicts
}

func normalizeGatewayMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", GatewayModeDisabled:
		return GatewayModeDisabled, nil
	case GatewayModeSubnetAccess:
		return GatewayModeSubnetAccess, nil
	case GatewayModeEgressGateway:
		return GatewayModeEgressGateway, nil
	default:
		return "", fmt.Errorf("%w: mode must be disabled, subnet_access, or egress_gateway", ErrInvalidGatewayMode)
	}
}
