package reconcile

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/persistence/agentrepo"
	"github.com/zivego/wiregate/internal/persistence/peerrepo"
	"github.com/zivego/wiregate/internal/persistence/runtimesyncrepo"
	"github.com/zivego/wiregate/internal/wgcontrol"
	"github.com/zivego/wiregate/pkg/wgadapter"
)

var ErrPeerNotFound = errors.New("peer not found")
var ErrPeerStateConflict = errors.New("peer state conflict")
var ErrInvalidPeerCursor = errors.New("invalid peer cursor")

type PeerView struct {
	ID                string
	AgentID           string
	Hostname          string
	PublicKey         string
	AssignedAddress   string
	AllowedIPs        []string
	RuntimeAllowedIPs []string
	Status            string
	Drift             string
}

type ListPeersFilter struct {
	Status  string
	AgentID string
	Query   string
	Limit   int
	Cursor  string
}

type PeerViewPage struct {
	Peers      []PeerView
	NextCursor string
}

type Summary struct {
	PeerCount   int
	Drifted     int
	Status      string
	DriftStatus string
}

// Service orchestrates runtime reconciliation across intended and observed peer state.
type Service struct {
	peers       *peerrepo.Repo
	agents      *agentrepo.Repo
	wg          *wgcontrol.Service
	runtimeSync *runtimesyncrepo.Repo
}

func NewService(peers *peerrepo.Repo, agents *agentrepo.Repo, wg *wgcontrol.Service, runtimeSync *runtimesyncrepo.Repo) *Service {
	return &Service{
		peers:       peers,
		agents:      agents,
		wg:          wg,
		runtimeSync: runtimeSync,
	}
}

func (s *Service) ListPeers(ctx context.Context, filter ListPeersFilter) ([]PeerView, error) {
	page, err := s.ListPeersPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Peers, nil
}

func (s *Service) ListPeersPage(ctx context.Context, filter ListPeersFilter) (PeerViewPage, error) {
	repoFilter := peerrepo.ListFilter{
		Status:  filter.Status,
		AgentID: filter.AgentID,
		Query:   filter.Query,
		Limit:   filter.Limit,
	}
	if strings.TrimSpace(filter.Cursor) != "" {
		cursorTime, cursorID, err := decodePeerCursor(filter.Cursor)
		if err != nil {
			return PeerViewPage{}, ErrInvalidPeerCursor
		}
		repoFilter.CursorTime = cursorTime
		repoFilter.CursorID = cursorID
	}

	records, hasMore, err := s.peers.ListPage(ctx, repoFilter)
	if err != nil {
		return PeerViewPage{}, fmt.Errorf("reconcile list peers: %w", err)
	}

	runtimeMap, err := s.runtimePeerMap(ctx)
	if err != nil {
		return PeerViewPage{}, err
	}

	agentIDs := make([]string, 0, len(records))
	for _, record := range records {
		agentIDs = append(agentIDs, record.AgentID)
	}
	agentsByID, err := s.agents.FindByIDs(ctx, agentIDs)
	if err != nil {
		return PeerViewPage{}, fmt.Errorf("reconcile list peers agent lookup: %w", err)
	}

	out := make([]PeerView, 0, len(records))
	for _, record := range records {
		view := s.toPeerView(record, runtimeMap[record.ID], agentsByID[record.AgentID])
		out = append(out, view)
	}
	page := PeerViewPage{Peers: out}
	if hasMore && len(records) > 0 {
		page.NextCursor = encodePeerCursor(records[len(records)-1])
	}
	return page, nil
}

func (s *Service) GetPeer(ctx context.Context, peerID string) (PeerView, error) {
	record, err := s.peers.FindByID(ctx, peerID)
	if err != nil {
		return PeerView{}, fmt.Errorf("reconcile get peer: %w", err)
	}
	if record == nil {
		return PeerView{}, ErrPeerNotFound
	}

	runtimeMap, err := s.runtimePeerMap(ctx)
	if err != nil {
		return PeerView{}, err
	}
	agent, err := s.agents.FindByID(ctx, record.AgentID)
	if err != nil {
		return PeerView{}, fmt.Errorf("reconcile get peer load agent: %w", err)
	}
	var resolvedAgent agentrepo.Agent
	if agent != nil {
		resolvedAgent = *agent
	}
	return s.toPeerView(*record, runtimeMap[peerID], resolvedAgent), nil
}

func encodePeerCursor(peer peerrepo.Peer) string {
	payload, _ := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
		ID        string `json:"id"`
	}{
		CreatedAt: peer.CreatedAt.UTC().Format(time.RFC3339),
		ID:        peer.ID,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodePeerCursor(raw string) (time.Time, string, error) {
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

func (s *Service) toPeerView(record peerrepo.Peer, runtime wgadapter.PeerState, agent agentrepo.Agent) PeerView {
	view := PeerView{
		ID:              record.ID,
		AgentID:         record.AgentID,
		PublicKey:       record.PublicKey,
		AssignedAddress: record.AssignedAddress,
		AllowedIPs:      slices.Clone(record.AllowedIPs),
		Status:          record.Status,
		Drift:           classifyDrift(record.AllowedIPs, runtime),
	}
	if agent.ID != "" {
		view.Hostname = agent.Hostname
	}
	if runtime.PeerID != "" {
		view.RuntimeAllowedIPs = slices.Clone(runtime.AllowedIPs)
	}
	return view
}

func (s *Service) ReconcilePeer(ctx context.Context, peerID string) (PeerView, error) {
	record, err := s.peers.FindByID(ctx, peerID)
	if err != nil {
		return PeerView{}, fmt.Errorf("reconcile peer find: %w", err)
	}
	if record == nil {
		return PeerView{}, ErrPeerNotFound
	}
	if record.Status == "disabled" || record.Status == "revoked" {
		return PeerView{}, ErrPeerStateConflict
	}
	now := time.Now().UTC()

	if err := s.wg.ApplyPeer(ctx, wgadapter.ApplyPeerInput{
		PeerID:     record.ID,
		PublicKey:  record.PublicKey,
		AllowedIPs: record.AllowedIPs,
		Action:     "apply",
	}); err != nil {
		s.persistRuntimeSyncState(ctx, record, "reconcile_failed", now, nil, map[string]any{
			"source":               "reconcile",
			"error":                err.Error(),
			"intended_allowed_ips": record.AllowedIPs,
		})
		return PeerView{}, fmt.Errorf("reconcile peer apply: %w", err)
	}
	if err := s.peers.UpdateStatus(ctx, record.ID, "active"); err != nil {
		return PeerView{}, fmt.Errorf("reconcile peer update status: %w", err)
	}
	record.Status = "active"

	runtimeMap, err := s.runtimePeerMap(ctx)
	if err != nil {
		return PeerView{}, err
	}
	s.persistRuntimeSyncState(ctx, record, "in_sync", now, &now, map[string]any{
		"source":               "reconcile",
		"intended_allowed_ips": record.AllowedIPs,
		"runtime_allowed_ips":  record.AllowedIPs,
	})
	agent, err := s.agents.FindByID(ctx, record.AgentID)
	if err != nil {
		return PeerView{}, fmt.Errorf("reconcile peer load agent: %w", err)
	}
	var resolvedAgent agentrepo.Agent
	if agent != nil {
		resolvedAgent = *agent
	}
	return s.toPeerView(*record, runtimeMap[peerID], resolvedAgent), nil
}

func (s *Service) Summarize(ctx context.Context) (Summary, error) {
	records, err := s.peers.List(ctx, peerrepo.ListFilter{})
	if err != nil {
		return Summary{}, fmt.Errorf("reconcile summarize list peers: %w", err)
	}
	runtimeMap, err := s.runtimePeerMap(ctx)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		PeerCount:   len(records),
		Status:      "ready",
		DriftStatus: "in_sync",
	}
	for _, record := range records {
		if classifyDrift(record.AllowedIPs, runtimeMap[record.ID]) != "in_sync" {
			summary.Drifted++
		}
	}
	if summary.Drifted > 0 {
		summary.DriftStatus = "drifted"
	}
	return summary, nil
}

func (s *Service) runtimePeerMap(ctx context.Context) (map[string]wgadapter.PeerState, error) {
	runtimePeers, err := s.wg.ListPeers(ctx)
	if err != nil {
		return nil, fmt.Errorf("reconcile runtime peers: %w", err)
	}
	runtimeMap := make(map[string]wgadapter.PeerState, len(runtimePeers))
	for _, peer := range runtimePeers {
		runtimeMap[peer.PeerID] = peer
	}
	return runtimeMap, nil
}

func classifyDrift(intended []string, runtime wgadapter.PeerState) string {
	if runtime.PeerID == "" && len(intended) == 0 {
		return "in_sync"
	}
	if runtime.PeerID == "" {
		return "missing_runtime"
	}
	expected := slices.Clone(intended)
	actual := slices.Clone(runtime.AllowedIPs)
	slices.Sort(expected)
	slices.Sort(actual)
	if slices.Equal(expected, actual) {
		return "in_sync"
	}
	return "allowed_ips_mismatch"
}

func (s *Service) persistRuntimeSyncState(ctx context.Context, peer *peerrepo.Peer, driftState string, observedAt time.Time, reconciledAt *time.Time, details map[string]any) {
	if s.runtimeSync == nil || peer == nil {
		return
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return
	}
	_ = s.runtimeSync.Upsert(ctx, runtimesyncrepo.State{
		ID:               peer.ID,
		PeerID:           peer.ID,
		DriftState:       driftState,
		LastObservedAt:   observedAt,
		LastReconciledAt: reconciledAt,
		DetailsJSON:      string(payload),
	})
}
