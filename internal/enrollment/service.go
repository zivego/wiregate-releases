package enrollment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/persistence/agentrepo"
	"github.com/zivego/wiregate/internal/persistence/enrollmenttokenrepo"
	"github.com/zivego/wiregate/internal/persistence/peerrepo"
	"github.com/zivego/wiregate/internal/persistence/runtimesyncrepo"
	"github.com/zivego/wiregate/pkg/wgconfig"
)

const (
	ModelA = "A"
	ModelB = "B"

	StatusIssued  = "issued"
	StatusUsed    = "used"
	StatusRevoked = "revoked"
	StatusExpired = "expired"

	defaultModelATTL = 10 * time.Minute
	defaultModelBTTL = 30 * time.Minute
	maxModelATTL     = 60 * time.Minute
	maxModelBTTL     = 24 * time.Hour
)

var (
	ErrInvalidTokenRequest      = errors.New("invalid token request")
	ErrInvalidEnrollmentRequest = errors.New("invalid enrollment request")
	ErrInvalidEnrollmentToken   = errors.New("invalid enrollment token")
	ErrInvalidAgentToken        = errors.New("invalid agent token")
	ErrTokenNotFound            = errors.New("token not found")
	ErrTokenStateConflict       = errors.New("token state conflict")
	ErrEnrollmentConflict       = errors.New("enrollment conflict")
	ErrAgentNotFound            = errors.New("agent not found")
	ErrAgentStateConflict       = errors.New("agent state conflict")
	ErrInvalidAgentCursor       = errors.New("invalid agent cursor")
	ErrInvalidTokenCursor       = errors.New("invalid enrollment token cursor")
	scopePattern                = regexp.MustCompile(`^[a-z0-9:_-]+$`)
	hostnamePattern             = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)
	publicKeyPattern            = regexp.MustCompile(`^[A-Za-z0-9+/=._:-]{16,256}$`)
)

// Token is the domain representation of an enrollment token.
type Token struct {
	ID              string
	Model           string
	Scope           string
	Status          string
	BoundIdentity   string
	AccessPolicyIDs []string
	ExpiresAt       time.Time
	UsedAt          *time.Time
	RevokedAt       *time.Time
	CreatedByUserID string
	CreatedAt       time.Time
}

// CreateTokenInput defines server-side issuance parameters.
type CreateTokenInput struct {
	Model           string
	Scope           string
	BoundIdentity   string
	AccessPolicyIDs []string
	TTL             time.Duration
	CreatedByUserID string
}

// PerformEnrollmentInput defines one token-based agent enrollment attempt.
type PerformEnrollmentInput struct {
	Token     string
	Hostname  string
	Platform  string
	PublicKey string
}

// ListAgentsFilter limits agent inventory results.
type ListAgentsFilter struct {
	Status   string
	Platform string
	Query    string
	Limit    int
	Cursor   string
}

type ListTokensFilter struct {
	Limit  int
	Cursor string
}

// PeerInventory is the minimal read model for one peer.
type PeerInventory struct {
	ID              string
	PublicKey       string
	AssignedAddress string
	AllowedIPs      []string
	Status          string
	CreatedAt       time.Time
}

// AgentInventory is the minimal read model for one agent.
type AgentInventory struct {
	ID                        string
	Hostname                  string
	Platform                  string
	Status                    string
	GatewayMode               string
	LastSeenAt                *time.Time
	ReportedVersion           string
	ReportedConfigFingerprint string
	LastApplyStatus           string
	LastApplyError            string
	LastAppliedAt             *time.Time
	CreatedAt                 time.Time
	Peer                      *PeerInventory
}

type AgentInventoryPage struct {
	Agents     []AgentInventory
	NextCursor string
}

type TokenPage struct {
	Tokens     []Token
	NextCursor string
}

// WireGuardConfig is the agent-side config payload derived from server-authoritative state.
type WireGuardConfig struct {
	InterfaceAddress string
	ServerEndpoint   string
	ServerPublicKey  string
	AllowedIPs       []string
	DNSServers       []string
	DNSSearchDomains []string
}

// PerformEnrollmentResult contains the token and created inventory entities.
type PerformEnrollmentResult struct {
	Token           Token
	Agent           AgentInventory
	AgentAuthToken  string
	WireGuardConfig *WireGuardConfig
}

// CheckInAgentInput defines one agent heartbeat request.
type CheckInAgentInput struct {
	AgentID           string
	Token             string
	Version           string
	LocalState        AgentLocalState
	RotationPublicKey string
}

type AgentLocalState struct {
	ReportedConfigFingerprint string
	LastApplyStatus           string
	LastApplyError            string
	LastAppliedAt             *time.Time
}

// CheckInAgentResult contains desired state returned to the agent.
type CheckInAgentResult struct {
	Agent               AgentInventory
	ReconfigureRequired bool
	DesiredState        string
	RotationRequired    bool
	Version             string
	CheckedInAt         time.Time
	WireGuardConfig     *WireGuardConfig
}

// BootstrapConfig contains server-side WireGuard bootstrap metadata for agents.
type BootstrapConfig struct {
	ServerEndpoint  string
	ServerPublicKey string
	ClientCIDR      string
}

type DNSSettings struct {
	Enabled       bool
	Servers       []string
	SearchDomains []string
}

// Service owns enrollment token lifecycle rules.
type Service struct {
	tokenRepo    *enrollmenttokenrepo.Repo
	agentRepo    *agentrepo.Repo
	peerRepo     *peerrepo.Repo
	runtimeSync  *runtimesyncrepo.Repo
	bootstrap    BootstrapConfig
	policyBinder policyBinder
	dnsProvider  dnsConfigProvider
}

type policyBinder interface {
	ValidatePolicyIDs(ctx context.Context, policyIDs []string) error
	AssignPoliciesToAgent(ctx context.Context, agentID string, policyIDs []string) error
	ListPolicyIDsForAgent(ctx context.Context, agentID string) ([]string, error)
}

type dnsConfigProvider interface {
	GetAgentDNSSettings(ctx context.Context) (DNSSettings, error)
}

const (
	agentStatusEnrolled = "enrolled"
	agentStatusDisabled = "disabled"
	agentStatusRevoked  = "revoked"

	peerStatusPlanned         = "planned"
	peerStatusActive          = "active"
	peerStatusDisabled        = "disabled"
	peerStatusRevoked         = "revoked"
	peerStatusRotationPending = "rotation_pending"

	desiredStateActive   = "active"
	desiredStateDisabled = "disabled"
)

func NewService(tokenRepo *enrollmenttokenrepo.Repo, agentRepo *agentrepo.Repo, peerRepo *peerrepo.Repo, bootstrap BootstrapConfig, policyBinder policyBinder, runtimeSync *runtimesyncrepo.Repo) *Service {
	return &Service{
		tokenRepo:    tokenRepo,
		agentRepo:    agentRepo,
		peerRepo:     peerRepo,
		runtimeSync:  runtimeSync,
		bootstrap:    bootstrap,
		policyBinder: policyBinder,
	}
}

func (s *Service) SetDNSProvider(provider dnsConfigProvider) {
	if s == nil {
		return
	}
	s.dnsProvider = provider
}

// CreateToken issues a new enrollment token and returns the raw value once.
func (s *Service) CreateToken(ctx context.Context, in CreateTokenInput) (Token, string, string, error) {
	if s == nil || s.tokenRepo == nil {
		return Token{}, "", "", fmt.Errorf("enrollment create token: repo is not configured")
	}

	model, err := normalizeModel(in.Model)
	if err != nil {
		return Token{}, "", "", err
	}
	scope, err := normalizeScope(in.Scope)
	if err != nil {
		return Token{}, "", "", err
	}
	if in.CreatedByUserID == "" {
		return Token{}, "", "", fmt.Errorf("%w: created_by_user_id is required", ErrInvalidTokenRequest)
	}

	boundIdentity := in.BoundIdentity
	if model == ModelB && boundIdentity == "" {
		return Token{}, "", "", fmt.Errorf("%w: bound_identity is required for model B", ErrInvalidTokenRequest)
	}
	if model == ModelA && boundIdentity != "" {
		return Token{}, "", "", fmt.Errorf("%w: bound_identity is not allowed for model A", ErrInvalidTokenRequest)
	}
	if len(in.AccessPolicyIDs) > 0 && s.policyBinder != nil {
		if err := s.policyBinder.ValidatePolicyIDs(ctx, in.AccessPolicyIDs); err != nil {
			return Token{}, "", "", err
		}
	}

	ttl, warning, err := resolveTTL(model, in.TTL)
	if err != nil {
		return Token{}, "", "", err
	}

	id, err := newID()
	if err != nil {
		return Token{}, "", "", fmt.Errorf("enrollment create token id: %w", err)
	}
	rawToken, tokenHash, err := generateToken()
	if err != nil {
		return Token{}, "", "", fmt.Errorf("enrollment create token secret: %w", err)
	}

	now := time.Now().UTC()
	token := enrollmenttokenrepo.Token{
		ID:              id,
		TokenHash:       tokenHash,
		Model:           model,
		Scope:           scope,
		Status:          StatusIssued,
		BoundIdentity:   boundIdentity,
		ExpiresAt:       now.Add(ttl),
		CreatedByUserID: in.CreatedByUserID,
		CreatedAt:       now,
	}
	if err := s.tokenRepo.Insert(ctx, token); err != nil {
		return Token{}, "", "", fmt.Errorf("enrollment create token: %w", err)
	}
	if err := s.tokenRepo.BindPolicies(ctx, token.ID, uniqueSortedStrings(in.AccessPolicyIDs), now); err != nil {
		return Token{}, "", "", fmt.Errorf("enrollment create token bind policies: %w", err)
	}

	domainToken, err := s.toDomainToken(ctx, token)
	if err != nil {
		return Token{}, "", "", err
	}
	return domainToken, rawToken, warning, nil
}

// ListTokens returns issued tokens ordered from newest to oldest.
func (s *Service) ListTokens(ctx context.Context) ([]Token, error) {
	page, err := s.ListTokensPage(ctx, ListTokensFilter{})
	if err != nil {
		return nil, err
	}
	return page.Tokens, nil
}

func (s *Service) ListTokensPage(ctx context.Context, filter ListTokensFilter) (TokenPage, error) {
	if s == nil || s.tokenRepo == nil {
		return TokenPage{}, nil
	}

	repoFilter := enrollmenttokenrepo.ListFilter{Limit: filter.Limit}
	if strings.TrimSpace(filter.Cursor) != "" {
		cursorTime, cursorID, err := decodeTokenCursor(filter.Cursor)
		if err != nil {
			return TokenPage{}, ErrInvalidTokenCursor
		}
		repoFilter.CursorTime = cursorTime
		repoFilter.CursorID = cursorID
	}

	tokens, hasMore, err := s.tokenRepo.ListPage(ctx, repoFilter)
	if err != nil {
		return TokenPage{}, fmt.Errorf("enrollment list tokens: %w", err)
	}

	out := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		domainToken, err := s.toDomainToken(ctx, token)
		if err != nil {
			return TokenPage{}, err
		}
		out = append(out, domainToken)
	}
	page := TokenPage{Tokens: out}
	if hasMore && len(tokens) > 0 {
		page.NextCursor = encodeTokenCursor(tokens[len(tokens)-1])
	}
	return page, nil
}

// RevokeToken revokes an issued token.
func (s *Service) RevokeToken(ctx context.Context, id string) (Token, error) {
	if s == nil || s.tokenRepo == nil {
		return Token{}, fmt.Errorf("enrollment revoke token: repo is not configured")
	}

	token, err := s.tokenRepo.FindByID(ctx, id)
	if err != nil {
		return Token{}, fmt.Errorf("enrollment revoke token: %w", err)
	}
	if token == nil {
		return Token{}, ErrTokenNotFound
	}

	effective := effectiveStatus(*token)
	if effective != StatusIssued {
		return Token{}, fmt.Errorf("%w: token is %s", ErrTokenStateConflict, effective)
	}

	now := time.Now().UTC()
	if err := s.tokenRepo.Revoke(ctx, id, now); err != nil {
		return Token{}, fmt.Errorf("enrollment revoke token: %w", err)
	}
	token.Status = StatusRevoked
	token.RevokedAt = &now

	domainToken, err := s.toDomainToken(ctx, *token)
	if err != nil {
		return Token{}, err
	}
	return domainToken, nil
}

// PerformEnrollment consumes a token, creates an agent and peer, and marks the token used.
func (s *Service) PerformEnrollment(ctx context.Context, in PerformEnrollmentInput) (PerformEnrollmentResult, error) {
	var result PerformEnrollmentResult
	if s == nil || s.tokenRepo == nil || s.agentRepo == nil || s.peerRepo == nil {
		return result, fmt.Errorf("enrollment perform: repos are not configured")
	}

	if in.Token == "" {
		return result, fmt.Errorf("%w: token is required", ErrInvalidEnrollmentRequest)
	}
	if !hostnamePattern.MatchString(in.Hostname) {
		return result, fmt.Errorf("%w: hostname must match %s", ErrInvalidEnrollmentRequest, hostnamePattern.String())
	}
	if in.Platform != "windows" && in.Platform != "linux" {
		return result, fmt.Errorf("%w: platform must be windows or linux", ErrInvalidEnrollmentRequest)
	}
	if !publicKeyPattern.MatchString(in.PublicKey) {
		return result, fmt.Errorf("%w: public_key format is invalid", ErrInvalidEnrollmentRequest)
	}

	token, err := s.tokenRepo.FindByHash(ctx, hashToken(in.Token))
	if err != nil {
		return result, fmt.Errorf("enrollment perform: %w", err)
	}
	if token == nil {
		return result, ErrInvalidEnrollmentToken
	}
	result.Token, err = s.toDomainToken(ctx, *token)
	if err != nil {
		return result, err
	}

	switch effectiveStatus(*token) {
	case StatusRevoked, StatusUsed, StatusExpired:
		return result, fmt.Errorf("%w: token is %s", ErrTokenStateConflict, effectiveStatus(*token))
	}

	if token.Model == ModelB && token.BoundIdentity != in.Hostname {
		return result, fmt.Errorf("%w: hostname does not match bound identity", ErrEnrollmentConflict)
	}

	existingAgent, err := s.agentRepo.FindByHostname(ctx, in.Hostname)
	if err != nil {
		return result, fmt.Errorf("enrollment perform: %w", err)
	}
	if existingAgent != nil && existingAgent.Status != agentStatusRevoked {
		return result, fmt.Errorf("%w: hostname already enrolled", ErrEnrollmentConflict)
	}

	existingPeer, err := s.peerRepo.FindByPublicKey(ctx, in.PublicKey)
	if err != nil {
		return result, fmt.Errorf("enrollment perform: %w", err)
	}
	if existingPeer != nil {
		return result, fmt.Errorf("%w: public key already enrolled", ErrEnrollmentConflict)
	}

	now := time.Now().UTC()
	agentID, err := newID()
	if err != nil {
		return result, fmt.Errorf("enrollment perform agent id: %w", err)
	}
	peerID, err := newID()
	if err != nil {
		return result, fmt.Errorf("enrollment perform peer id: %w", err)
	}
	assignedAddress, err := s.allocatePeerAddress(ctx)
	if err != nil {
		return result, err
	}
	agentAuthToken, agentAuthTokenHash, err := generateToken()
	if err != nil {
		return result, fmt.Errorf("enrollment perform agent token: %w", err)
	}

	agent := agentrepo.Agent{
		ID:            agentID,
		Hostname:      in.Hostname,
		Platform:      in.Platform,
		Status:        agentStatusEnrolled,
		AuthTokenHash: agentAuthTokenHash,
		CreatedAt:     now,
	}
	if err := s.agentRepo.Insert(ctx, agent); err != nil {
		return result, fmt.Errorf("enrollment perform create agent: %w", err)
	}

	peer := &peerrepo.Peer{
		ID:              peerID,
		AgentID:         agentID,
		PublicKey:       in.PublicKey,
		AssignedAddress: assignedAddress,
		Status:          peerStatusPlanned,
		CreatedAt:       now,
	}
	if err := s.peerRepo.Insert(ctx, *peer); err != nil {
		return result, fmt.Errorf("enrollment perform create peer: %w", err)
	}

	policyIDs, err := s.tokenRepo.ListPolicyIDs(ctx, token.ID)
	if err != nil {
		return result, fmt.Errorf("enrollment perform list token policy ids: %w", err)
	}
	if len(policyIDs) > 0 && s.policyBinder != nil {
		if err := s.policyBinder.AssignPoliciesToAgent(ctx, agentID, policyIDs); err != nil {
			return result, fmt.Errorf("enrollment perform assign policies: %w", err)
		}
		peer, err = s.peerRepo.FindByAgentID(ctx, agentID)
		if err != nil {
			return result, fmt.Errorf("enrollment perform reload peer after policy assignment: %w", err)
		}
	}

	if err := s.tokenRepo.MarkUsed(ctx, token.ID, now); err != nil {
		return result, fmt.Errorf("enrollment perform mark token used: %w", err)
	}

	token.Status = StatusUsed
	token.UsedAt = &now
	result.Token, err = s.toDomainToken(ctx, *token)
	if err != nil {
		return result, err
	}
	result.Agent = AgentInventory{
		ID:        agent.ID,
		Hostname:  agent.Hostname,
		Platform:  agent.Platform,
		Status:    agent.Status,
		CreatedAt: agent.CreatedAt,
		Peer:      mapPeerInventory(peer),
	}
	result.AgentAuthToken = agentAuthToken
	result.WireGuardConfig, err = s.buildWireGuardConfig(ctx, peer)
	if err != nil {
		return result, fmt.Errorf("enrollment perform build config: %w", err)
	}
	return result, nil
}

// CheckInAgent authenticates one agent heartbeat and returns desired peer state.
func (s *Service) CheckInAgent(ctx context.Context, in CheckInAgentInput) (CheckInAgentResult, error) {
	var result CheckInAgentResult
	if s == nil || s.agentRepo == nil || s.peerRepo == nil {
		return result, fmt.Errorf("enrollment check in: repos are not configured")
	}
	if in.AgentID == "" || in.Token == "" {
		return result, ErrInvalidAgentToken
	}

	agent, err := s.agentRepo.FindByID(ctx, in.AgentID)
	if err != nil {
		return result, fmt.Errorf("enrollment check in: %w", err)
	}
	if agent == nil {
		return result, ErrAgentNotFound
	}
	if agent.AuthTokenHash == "" || hashToken(in.Token) != agent.AuthTokenHash {
		return result, ErrInvalidAgentToken
	}
	if agent.Status == agentStatusRevoked {
		return result, ErrInvalidAgentToken
	}

	now := time.Now().UTC()
	localState, err := normalizeAgentLocalState(in.LocalState)
	if err != nil {
		return result, err
	}
	if err := s.agentRepo.UpdateCheckIn(ctx, agent.ID, agentrepo.CheckInStatus{
		LastSeenAt:                now,
		ReportedVersion:           strings.TrimSpace(in.Version),
		ReportedConfigFingerprint: localState.ReportedConfigFingerprint,
		LastApplyStatus:           localState.LastApplyStatus,
		LastApplyError:            localState.LastApplyError,
		LastAppliedAt:             localState.LastAppliedAt,
	}); err != nil {
		return result, fmt.Errorf("enrollment check in update last seen: %w", err)
	}
	agent.LastSeenAt = &now
	agent.ReportedVersion = strings.TrimSpace(in.Version)
	agent.ReportedConfigFingerprint = localState.ReportedConfigFingerprint
	agent.LastApplyStatus = localState.LastApplyStatus
	agent.LastApplyError = localState.LastApplyError
	agent.LastAppliedAt = localState.LastAppliedAt

	peer, err := s.peerRepo.FindByAgentID(ctx, agent.ID)
	if err != nil {
		return result, fmt.Errorf("enrollment check in peer lookup: %w", err)
	}
	if peer != nil && agent.Status != agentStatusRevoked && strings.TrimSpace(in.RotationPublicKey) != "" {
		updatedPeer, err := s.completeRotation(ctx, agent, peer, strings.TrimSpace(in.RotationPublicKey))
		if err != nil {
			return result, err
		}
		peer = updatedPeer
	}
	desiredConfig, err := s.buildWireGuardConfig(ctx, peer)
	if err != nil {
		return result, fmt.Errorf("enrollment check in build config: %w", err)
	}
	desiredState := desiredStateForAgent(*agent)

	result = CheckInAgentResult{
		Agent:               mapAgentInventory(*agent, peer),
		ReconfigureRequired: requiresAgentReconfigure(agent, peer, desiredConfig, localState),
		DesiredState:        desiredState,
		RotationRequired:    peer != nil && peer.Status == peerStatusRotationPending,
		Version:             in.Version,
		CheckedInAt:         now,
		WireGuardConfig:     desiredConfig,
	}
	if s.runtimeSync != nil && peer != nil {
		s.persistRuntimeSyncCheckIn(ctx, agent, peer, desiredConfig, localState, result)
	}
	return result, nil
}

func (s *Service) persistRuntimeSyncCheckIn(ctx context.Context, agent *agentrepo.Agent, peer *peerrepo.Peer, desiredConfig *WireGuardConfig, localState AgentLocalState, result CheckInAgentResult) {
	detailsJSON, err := json.Marshal(map[string]any{
		"source":                      "agent_check_in",
		"agent_id":                    agent.ID,
		"agent_status":                agent.Status,
		"peer_status":                 peer.Status,
		"reported_version":            agent.ReportedVersion,
		"last_apply_status":           localState.LastApplyStatus,
		"last_apply_error":            localState.LastApplyError,
		"reported_config_fingerprint": localState.ReportedConfigFingerprint,
		"desired_config_fingerprint":  wireGuardConfigFingerprint(desiredConfig),
		"reconfigure_required":        result.ReconfigureRequired,
		"rotation_required":           result.RotationRequired,
		"desired_state":               result.DesiredState,
	})
	if err != nil {
		return
	}
	_ = s.runtimeSync.Upsert(ctx, runtimesyncrepo.State{
		ID:             peer.ID,
		PeerID:         peer.ID,
		DriftState:     classifyRuntimeSyncDrift(agent, peer, desiredConfig, localState),
		LastObservedAt: result.CheckedInAt,
		DetailsJSON:    string(detailsJSON),
	})
}

func (s *Service) allocatePeerAddress(ctx context.Context) (string, error) {
	if s.bootstrap.ClientCIDR == "" {
		return "", nil
	}
	prefix, err := netip.ParsePrefix(s.bootstrap.ClientCIDR)
	if err != nil {
		return "", fmt.Errorf("enrollment allocate peer address parse cidr: %w", err)
	}
	prefix = prefix.Masked()

	peers, err := s.peerRepo.List(ctx, peerrepo.ListFilter{})
	if err != nil {
		return "", fmt.Errorf("enrollment allocate peer address list peers: %w", err)
	}
	used := make(map[netip.Addr]struct{}, len(peers))
	for _, peer := range peers {
		if peer.AssignedAddress == "" {
			continue // no address assigned or reclaimed
		}
		if peer.Status == peerStatusRevoked {
			continue // revoked peers have reclaimed IPs
		}
		assignedPrefix, err := netip.ParsePrefix(peer.AssignedAddress)
		if err != nil {
			continue
		}
		used[assignedPrefix.Addr()] = struct{}{}
	}

	base := prefix.Addr()
	broadcast := netip.Addr{}
	isIPv4 := base.Is4() && !base.Is4In6()
	if isIPv4 && prefix.Bits() < 31 {
		broadcast = lastAddress(prefix)
	}
	for candidate := base.Next(); prefix.Contains(candidate); candidate = candidate.Next() {
		if _, exists := used[candidate]; exists {
			continue
		}
		if isIPv4 && broadcast.IsValid() && candidate == broadcast {
			continue
		}
		suffix := "/32"
		if !isIPv4 {
			suffix = "/128"
		}
		return candidate.String() + suffix, nil
	}
	return "", fmt.Errorf("enrollment allocate peer address: no free addresses in %s", prefix.String())
}

func lastAddress(prefix netip.Prefix) netip.Addr {
	addr := prefix.Masked().Addr()
	raw := addr.As16()
	for bit := prefix.Bits(); bit < addr.BitLen(); bit++ {
		byteIdx := bit / 8
		bitIdx := 7 - (bit % 8)
		raw[byteIdx] |= 1 << bitIdx
	}
	if addr.Is4() {
		return netip.AddrFrom4([4]byte(raw[12:16]))
	}
	return netip.AddrFrom16(raw)
}

func (s *Service) buildWireGuardConfig(ctx context.Context, peer *peerrepo.Peer) (*WireGuardConfig, error) {
	if peer == nil || peer.AssignedAddress == "" || s.bootstrap.ServerEndpoint == "" || s.bootstrap.ServerPublicKey == "" {
		return nil, nil
	}
	cfg := &WireGuardConfig{
		InterfaceAddress: peer.AssignedAddress,
		ServerEndpoint:   s.bootstrap.ServerEndpoint,
		ServerPublicKey:  s.bootstrap.ServerPublicKey,
		AllowedIPs:       slices.Clone(peer.AllowedIPs),
	}
	if s.dnsProvider != nil {
		dnsSettings, err := s.dnsProvider.GetAgentDNSSettings(ctx)
		if err != nil {
			return nil, err
		}
		if dnsSettings.Enabled {
			cfg.DNSServers = slices.Clone(dnsSettings.Servers)
			cfg.DNSSearchDomains = slices.Clone(dnsSettings.SearchDomains)
		}
	}
	return cfg, nil
}

func requiresAgentReconfigure(agent *agentrepo.Agent, peer *peerrepo.Peer, cfg *WireGuardConfig, localState AgentLocalState) bool {
	if agent != nil && agent.Status == agentStatusDisabled {
		return localState.LastApplyStatus != "disabled"
	}
	if peer != nil && peer.Status != peerStatusActive {
		return true
	}
	switch localState.LastApplyStatus {
	case "apply_failed", "drifted":
		return true
	}
	desiredFingerprint := wireGuardConfigFingerprint(cfg)
	if desiredFingerprint == "" {
		return false
	}
	return localState.ReportedConfigFingerprint != desiredFingerprint
}

func classifyRuntimeSyncDrift(agent *agentrepo.Agent, peer *peerrepo.Peer, cfg *WireGuardConfig, localState AgentLocalState) string {
	switch localState.LastApplyStatus {
	case "apply_failed":
		return "apply_failed"
	case "drifted":
		return "drifted"
	case "staged":
		return "pending_apply"
	}
	if peer == nil {
		return "missing_runtime"
	}
	if peer.Status == peerStatusRotationPending {
		return "rotation_pending"
	}
	if peer.Status != peerStatusActive {
		return "pending_reconcile"
	}
	desiredFingerprint := wireGuardConfigFingerprint(cfg)
	if desiredFingerprint != "" && localState.ReportedConfigFingerprint != "" && localState.ReportedConfigFingerprint != desiredFingerprint {
		return "config_outdated"
	}
	if agent != nil && agent.Status == agentStatusDisabled && localState.LastApplyStatus != "disabled" {
		return "pending_disable"
	}
	return "in_sync"
}

func desiredStateForAgent(agent agentrepo.Agent) string {
	if agent.Status == agentStatusDisabled {
		return desiredStateDisabled
	}
	return desiredStateActive
}

// ListAgents returns agent inventory with optional filtering.
func (s *Service) ListAgents(ctx context.Context, filter ListAgentsFilter) ([]AgentInventory, error) {
	page, err := s.ListAgentsPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Agents, nil
}

func (s *Service) ListAgentsPage(ctx context.Context, filter ListAgentsFilter) (AgentInventoryPage, error) {
	if s == nil || s.agentRepo == nil || s.peerRepo == nil {
		return AgentInventoryPage{}, nil
	}

	repoFilter := agentrepo.ListFilter{
		Status:   filter.Status,
		Platform: filter.Platform,
		Query:    filter.Query,
		Limit:    filter.Limit,
	}
	if strings.TrimSpace(filter.Cursor) != "" {
		cursorTime, cursorID, err := decodeAgentCursor(filter.Cursor)
		if err != nil {
			return AgentInventoryPage{}, ErrInvalidAgentCursor
		}
		repoFilter.CursorTime = cursorTime
		repoFilter.CursorID = cursorID
	}
	agents, hasMore, err := s.agentRepo.ListPage(ctx, repoFilter)
	if err != nil {
		return AgentInventoryPage{}, fmt.Errorf("enrollment list agents: %w", err)
	}

	agentIDs := make([]string, 0, len(agents))
	for _, agent := range agents {
		agentIDs = append(agentIDs, agent.ID)
	}
	peersByAgentID, err := s.peerRepo.FindByAgentIDs(ctx, agentIDs)
	if err != nil {
		return AgentInventoryPage{}, fmt.Errorf("enrollment list agents peer lookup: %w", err)
	}

	out := make([]AgentInventory, 0, len(agents))
	for _, agent := range agents {
		var peer *peerrepo.Peer
		if found, ok := peersByAgentID[agent.ID]; ok {
			peer = &found
		}
		out = append(out, mapAgentInventory(agent, peer))
	}
	page := AgentInventoryPage{Agents: out}
	if hasMore && len(agents) > 0 {
		page.NextCursor = encodeAgentCursor(agents[len(agents)-1])
	}
	return page, nil
}

// GetAgent returns one agent inventory record.
func (s *Service) GetAgent(ctx context.Context, agentID string) (AgentInventory, error) {
	if s == nil || s.agentRepo == nil || s.peerRepo == nil {
		return AgentInventory{}, fmt.Errorf("enrollment get agent: repos are not configured")
	}

	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment get agent: %w", err)
	}
	if agent == nil {
		return AgentInventory{}, ErrAgentNotFound
	}

	peer, err := s.peerRepo.FindByAgentID(ctx, agentID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment get agent peer lookup: %w", err)
	}
	return mapAgentInventory(*agent, peer), nil
}

func normalizeModel(model string) (string, error) {
	switch model {
	case ModelA, ModelB:
		return model, nil
	default:
		return "", fmt.Errorf("%w: model must be A or B", ErrInvalidTokenRequest)
	}
}

func normalizeScope(scope string) (string, error) {
	if scope == "" {
		return "", fmt.Errorf("%w: scope is required", ErrInvalidTokenRequest)
	}
	if !scopePattern.MatchString(scope) {
		return "", fmt.Errorf("%w: scope must match %s", ErrInvalidTokenRequest, scopePattern.String())
	}
	return scope, nil
}

func resolveTTL(model string, requested time.Duration) (time.Duration, string, error) {
	switch model {
	case ModelA:
		if requested == 0 {
			return defaultModelATTL, "Model A generic tokens carry higher risk. Keep TTL short and revoke immediately if exposure is suspected.", nil
		}
		if requested < time.Minute || requested > maxModelATTL {
			return 0, "", fmt.Errorf("%w: model A ttl must be between 1 and 60 minutes", ErrInvalidTokenRequest)
		}
		return requested, "Model A generic tokens carry higher risk. Keep TTL short and revoke immediately if exposure is suspected.", nil
	case ModelB:
		if requested == 0 {
			return defaultModelBTTL, "", nil
		}
		if requested < time.Minute || requested > maxModelBTTL {
			return 0, "", fmt.Errorf("%w: model B ttl must be between 1 minute and 24 hours", ErrInvalidTokenRequest)
		}
		return requested, "", nil
	default:
		return 0, "", fmt.Errorf("%w: unsupported model", ErrInvalidTokenRequest)
	}
}

func (s *Service) toDomainToken(ctx context.Context, token enrollmenttokenrepo.Token) (Token, error) {
	policyIDs, err := s.tokenRepo.ListPolicyIDs(ctx, token.ID)
	if err != nil {
		return Token{}, fmt.Errorf("enrollment to domain token policy ids: %w", err)
	}
	return Token{
		ID:              token.ID,
		Model:           token.Model,
		Scope:           token.Scope,
		Status:          effectiveStatus(token),
		BoundIdentity:   token.BoundIdentity,
		AccessPolicyIDs: policyIDs,
		ExpiresAt:       token.ExpiresAt,
		UsedAt:          token.UsedAt,
		RevokedAt:       token.RevokedAt,
		CreatedByUserID: token.CreatedByUserID,
		CreatedAt:       token.CreatedAt,
	}, nil
}

func effectiveStatus(token enrollmenttokenrepo.Token) string {
	if token.Status == StatusRevoked || token.RevokedAt != nil {
		return StatusRevoked
	}
	if token.Status == StatusUsed || token.UsedAt != nil {
		return StatusUsed
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return StatusExpired
	}
	return StatusIssued
}

func mapAgentInventory(agent agentrepo.Agent, peer *peerrepo.Peer) AgentInventory {
	out := AgentInventory{
		ID:                        agent.ID,
		Hostname:                  agent.Hostname,
		Platform:                  agent.Platform,
		Status:                    agent.Status,
		GatewayMode:               agent.GatewayMode,
		LastSeenAt:                agent.LastSeenAt,
		ReportedVersion:           agent.ReportedVersion,
		ReportedConfigFingerprint: agent.ReportedConfigFingerprint,
		LastApplyStatus:           agent.LastApplyStatus,
		LastApplyError:            agent.LastApplyError,
		LastAppliedAt:             agent.LastAppliedAt,
		CreatedAt:                 agent.CreatedAt,
	}
	if peer != nil {
		out.Peer = mapPeerInventory(peer)
	}
	return out
}

type agentCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeAgentCursor(agent agentrepo.Agent) string {
	payload, _ := json.Marshal(agentCursor{
		CreatedAt: agent.CreatedAt.UTC().Format(time.RFC3339),
		ID:        agent.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeAgentCursor(raw string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", err
	}
	var cursor agentCursor
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

func encodeTokenCursor(token enrollmenttokenrepo.Token) string {
	payload, _ := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}{
		CreatedAt: token.CreatedAt.UTC().Format(time.RFC3339),
		ID:        token.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeTokenCursor(raw string) (time.Time, string, error) {
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

func mapPeerInventory(peer *peerrepo.Peer) *PeerInventory {
	if peer == nil {
		return nil
	}
	return &PeerInventory{
		ID:              peer.ID,
		PublicKey:       peer.PublicKey,
		AssignedAddress: peer.AssignedAddress,
		AllowedIPs:      slices.Clone(peer.AllowedIPs),
		Status:          peer.Status,
		CreatedAt:       peer.CreatedAt,
	}
}

func (s *Service) ChangeAgentState(ctx context.Context, agentID, action string) (AgentInventory, error) {
	if s == nil || s.agentRepo == nil || s.peerRepo == nil {
		return AgentInventory{}, fmt.Errorf("enrollment change agent state: repos are not configured")
	}

	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment change agent state find agent: %w", err)
	}
	if agent == nil {
		return AgentInventory{}, ErrAgentNotFound
	}
	peer, err := s.peerRepo.FindByAgentID(ctx, agentID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment change agent state find peer: %w", err)
	}

	switch action {
	case "disable":
		if agent.Status == agentStatusDisabled {
			return AgentInventory{}, fmt.Errorf("%w: agent already disabled", ErrAgentStateConflict)
		}
		if agent.Status == agentStatusRevoked {
			return AgentInventory{}, fmt.Errorf("%w: revoked agent cannot be disabled", ErrAgentStateConflict)
		}
		agent.Status = agentStatusDisabled
		if err := s.agentRepo.UpdateStatus(ctx, agent.ID, agent.Status); err != nil {
			return AgentInventory{}, fmt.Errorf("enrollment change agent state disable agent: %w", err)
		}
		if peer != nil {
			peer.Status = peerStatusDisabled
			if err := s.peerRepo.UpdateStatus(ctx, peer.ID, peer.Status); err != nil {
				return AgentInventory{}, fmt.Errorf("enrollment change agent state disable peer: %w", err)
			}
		}
	case "enable":
		if agent.Status != agentStatusDisabled {
			return AgentInventory{}, fmt.Errorf("%w: only disabled agents can be enabled", ErrAgentStateConflict)
		}
		agent.Status = agentStatusEnrolled
		if err := s.agentRepo.UpdateStatus(ctx, agent.ID, agent.Status); err != nil {
			return AgentInventory{}, fmt.Errorf("enrollment change agent state enable agent: %w", err)
		}
		if peer != nil {
			peer.Status = peerStatusPlanned
			if err := s.peerRepo.UpdateStatus(ctx, peer.ID, peer.Status); err != nil {
				return AgentInventory{}, fmt.Errorf("enrollment change agent state enable peer: %w", err)
			}
		}
	case "revoke":
		if agent.Status == agentStatusRevoked {
			return AgentInventory{}, fmt.Errorf("%w: agent already revoked", ErrAgentStateConflict)
		}
		agent.Status = agentStatusRevoked
		agent.AuthTokenHash = ""
		if err := s.agentRepo.UpdateStatus(ctx, agent.ID, agent.Status); err != nil {
			return AgentInventory{}, fmt.Errorf("enrollment change agent state revoke agent: %w", err)
		}
		if err := s.agentRepo.ClearAuthToken(ctx, agent.ID); err != nil {
			return AgentInventory{}, fmt.Errorf("enrollment change agent state clear auth token: %w", err)
		}
		if peer != nil {
			peer.Status = peerStatusRevoked
			if err := s.peerRepo.UpdateStatus(ctx, peer.ID, peer.Status); err != nil {
				return AgentInventory{}, fmt.Errorf("enrollment change agent state revoke peer: %w", err)
			}
			if peer.AssignedAddress != "" {
				if err := s.peerRepo.ClearAssignedAddress(ctx, peer.ID); err != nil {
					return AgentInventory{}, fmt.Errorf("enrollment change agent state reclaim ip: %w", err)
				}
				peer.AssignedAddress = ""
			}
		}
	default:
		return AgentInventory{}, fmt.Errorf("%w: unsupported state action", ErrInvalidEnrollmentRequest)
	}

	return mapAgentInventory(*agent, peer), nil
}

func (s *Service) ReissueAgentEnrollment(ctx context.Context, agentID, createdByUserID string) (Token, string, string, error) {
	if s == nil || s.agentRepo == nil {
		return Token{}, "", "", fmt.Errorf("enrollment reissue: repos are not configured")
	}
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return Token{}, "", "", fmt.Errorf("enrollment reissue find agent: %w", err)
	}
	if agent == nil {
		return Token{}, "", "", ErrAgentNotFound
	}
	if agent.Status != agentStatusRevoked {
		return Token{}, "", "", fmt.Errorf("%w: only revoked agents can be reissued", ErrAgentStateConflict)
	}

	var policyIDs []string
	if s.policyBinder != nil {
		policies, err := s.policyBinder.ListPolicyIDsForAgent(ctx, agent.ID)
		if err != nil {
			return Token{}, "", "", fmt.Errorf("enrollment reissue list policies: %w", err)
		}
		policyIDs = policies
	}
	return s.CreateToken(ctx, CreateTokenInput{
		Model:           ModelB,
		Scope:           "enrollment",
		BoundIdentity:   agent.Hostname,
		AccessPolicyIDs: uniqueSortedStrings(policyIDs),
		TTL:             defaultModelBTTL,
		CreatedByUserID: createdByUserID,
	})
}

func (s *Service) MarkAgentRotationPending(ctx context.Context, agentID string) (AgentInventory, error) {
	if s == nil || s.agentRepo == nil || s.peerRepo == nil {
		return AgentInventory{}, fmt.Errorf("enrollment rotate agent: repos are not configured")
	}
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment rotate agent find agent: %w", err)
	}
	if agent == nil {
		return AgentInventory{}, ErrAgentNotFound
	}
	if agent.Status == agentStatusRevoked {
		return AgentInventory{}, fmt.Errorf("%w: revoked agent cannot rotate", ErrAgentStateConflict)
	}
	peer, err := s.peerRepo.FindByAgentID(ctx, agent.ID)
	if err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment rotate agent find peer: %w", err)
	}
	if peer == nil {
		return AgentInventory{}, fmt.Errorf("%w: peer not found for agent", ErrAgentNotFound)
	}
	peer.Status = peerStatusRotationPending
	if err := s.peerRepo.UpdateStatus(ctx, peer.ID, peer.Status); err != nil {
		return AgentInventory{}, fmt.Errorf("enrollment rotate agent mark peer: %w", err)
	}
	return mapAgentInventory(*agent, peer), nil
}

func (s *Service) completeRotation(ctx context.Context, agent *agentrepo.Agent, peer *peerrepo.Peer, rotationPublicKey string) (*peerrepo.Peer, error) {
	if peer.Status != peerStatusRotationPending {
		return peer, nil
	}
	if !publicKeyPattern.MatchString(rotationPublicKey) {
		return nil, fmt.Errorf("%w: public_key format is invalid", ErrInvalidEnrollmentRequest)
	}
	if rotationPublicKey == peer.PublicKey {
		return nil, fmt.Errorf("%w: rotation public key must differ from current key", ErrEnrollmentConflict)
	}
	existingPeer, err := s.peerRepo.FindByPublicKey(ctx, rotationPublicKey)
	if err != nil {
		return nil, fmt.Errorf("enrollment complete rotation find by public key: %w", err)
	}
	if existingPeer != nil && existingPeer.ID != peer.ID {
		return nil, fmt.Errorf("%w: public key already enrolled", ErrEnrollmentConflict)
	}
	peer.PublicKey = rotationPublicKey
	peer.Status = peerStatusPlanned
	if err := s.peerRepo.UpdatePublicKeyAndStatus(ctx, peer.ID, peer.PublicKey, peer.Status); err != nil {
		return nil, fmt.Errorf("enrollment complete rotation update peer: %w", err)
	}
	return peer, nil
}

func uniqueSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, value := range in {
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

func normalizeAgentLocalState(in AgentLocalState) (AgentLocalState, error) {
	state := AgentLocalState{
		ReportedConfigFingerprint: strings.TrimSpace(in.ReportedConfigFingerprint),
		LastApplyStatus:           strings.TrimSpace(in.LastApplyStatus),
		LastApplyError:            strings.TrimSpace(in.LastApplyError),
		LastAppliedAt:             in.LastAppliedAt,
	}
	switch state.LastApplyStatus {
	case "", "applied", "staged", "apply_failed", "drifted", "disabled":
	default:
		return AgentLocalState{}, fmt.Errorf("%w: unsupported last_apply_status", ErrInvalidEnrollmentRequest)
	}
	if len(state.LastApplyError) > 256 {
		state.LastApplyError = state.LastApplyError[:256]
	}
	return state, nil
}

func wireGuardConfigFingerprint(cfg *WireGuardConfig) string {
	if cfg == nil {
		return ""
	}
	return wgconfig.Fingerprint(cfg.InterfaceAddress, cfg.ServerEndpoint, cfg.ServerPublicKey, cfg.AllowedIPs, cfg.DNSServers, cfg.DNSSearchDomains)
}
