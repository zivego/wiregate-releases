package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/reconcile"
)

type peerResponse struct {
	ID                string   `json:"id"`
	AgentID           string   `json:"agent_id"`
	Hostname          string   `json:"hostname,omitempty"`
	PublicKey         string   `json:"public_key"`
	AssignedAddress   string   `json:"assigned_address,omitempty"`
	AllowedIPs        []string `json:"allowed_ips"`
	RuntimeAllowedIPs []string `json:"runtime_allowed_ips,omitempty"`
	Status            string   `json:"status"`
	Drift             string   `json:"drift"`
}

func (r *Router) handleListPeers(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	pageSize := 50
	pageSizeRaw := req.URL.Query().Get("page_size")
	if pageSizeRaw == "" {
		pageSizeRaw = req.URL.Query().Get("limit")
	}
	if pageSizeRaw != "" {
		parsed, err := strconv.Atoi(pageSizeRaw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "validation_failed", "page_size must be a positive integer")
			return
		}
		pageSize = parsed
	}

	page, err := r.reconcileService.ListPeersPage(req.Context(), reconcile.ListPeersFilter{
		Status:  req.URL.Query().Get("status"),
		AgentID: req.URL.Query().Get("agent_id"),
		Query:   req.URL.Query().Get("q"),
		Limit:   pageSize,
		Cursor:  req.URL.Query().Get("cursor"),
	})
	if errors.Is(err, reconcile.ErrInvalidPeerCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid peer cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list peers error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list peers")
		return
	}

	resp := make([]peerResponse, 0, len(page.Peers))
	for _, peer := range page.Peers {
		resp = append(resp, peerToResponse(peer))
	}
	writeJSON(w, http.StatusOK, map[string]any{"peers": resp, "next_cursor": page.NextCursor})
}

func (r *Router) handleGetPeer(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	peerID, action, valid := parsePeerAction(req.URL.Path)
	if !valid || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "peer not found")
		return
	}

	peer, err := r.reconcileService.GetPeer(req.Context(), peerID)
	if errors.Is(err, reconcile.ErrPeerNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "peer not found")
		return
	}
	if err != nil {
		r.logger.Printf("get peer error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load peer")
		return
	}
	writeJSON(w, http.StatusOK, peerToResponse(peer))
}

func (r *Router) handleReconcilePeer(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManagePolicies(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}

	peerID, action, valid := parsePeerAction(req.URL.Path)
	if !valid || action != "reconcile" {
		writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	peer, err := r.reconcileService.ReconcilePeer(req.Context(), peerID)
	if errors.Is(err, reconcile.ErrPeerNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "peer not found")
		return
	}
	if errors.Is(err, reconcile.ErrPeerStateConflict) {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("reconcile peer error: %v", err)
		writeError(w, http.StatusInternalServerError, "runtime_apply_failed", "failed to reconcile peer")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "peer.reconcile",
		ResourceType: "peer",
		ResourceID:   peer.ID,
		Result:       "success",
		Metadata: map[string]any{
			"agent_id":    peer.AgentID,
			"allowed_ips": peer.AllowedIPs,
			"drift":       peer.Drift,
			"runtime_ips": peer.RuntimeAllowedIPs,
			"peer_status": peer.Status,
		},
	})
	writeJSON(w, http.StatusOK, peerToResponse(peer))
}

func parsePeerAction(path string) (peerID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/peers/")
	parts := strings.Split(rest, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", false
		}
		return parts[0], "", true
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

func peerToResponse(peer reconcile.PeerView) peerResponse {
	return peerResponse{
		ID:                peer.ID,
		AgentID:           peer.AgentID,
		Hostname:          peer.Hostname,
		PublicKey:         peer.PublicKey,
		AssignedAddress:   peer.AssignedAddress,
		AllowedIPs:        peer.AllowedIPs,
		RuntimeAllowedIPs: peer.RuntimeAllowedIPs,
		Status:            peer.Status,
		Drift:             peer.Drift,
	}
}
