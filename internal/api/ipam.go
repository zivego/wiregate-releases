package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

// handleIPAMPoolGet dispatches GET /api/v1/ipam/pools/{id}[/reservations].
func (r *Router) handleIPAMPoolGet(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	if strings.HasSuffix(path, "/reservations") {
		r.handleListIPAMReservations(w, req)
		return
	}
	r.handleGetIPAMPool(w, req)
}

// handleIPAMPoolPost dispatches POST /api/v1/ipam/pools/{id}/allocate.
func (r *Router) handleIPAMPoolPost(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	if strings.HasSuffix(path, "/allocate") {
		r.handleAllocateIPAMAddress(w, req)
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "unknown IPAM endpoint")
}

// --- IPAM pools ---

func (r *Router) handleListIPAMPools(w http.ResponseWriter, req *http.Request) {
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	pools, err := r.ipamService.ListPools(req.Context())
	if err != nil {
		r.logger.Printf("ipam list pools error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list pools")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"pools": pools})
}

func (r *Router) handleCreateIPAMPool(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	var body struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
		Gateway     string `json:"gateway"`
	}
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if body.CIDR == "" {
		writeError(w, http.StatusBadRequest, "validation", "cidr is required")
		return
	}

	id := generateID("pool")
	pool, err := r.ipamService.CreatePool(req.Context(), id, body.CIDR, body.Description, body.Gateway)
	if err != nil {
		r.logger.Printf("ipam create pool error: %v", err)
		writeError(w, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, pool)
}

func (r *Router) handleGetIPAMPool(w http.ResponseWriter, req *http.Request) {
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	poolID := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	if strings.Contains(poolID, "/") {
		poolID = poolID[:strings.Index(poolID, "/")]
	}

	pool, err := r.ipamService.GetPool(req.Context(), poolID)
	if err != nil {
		r.logger.Printf("ipam get pool error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get pool")
		return
	}
	if pool == nil {
		writeError(w, http.StatusNotFound, "not_found", "pool not found")
		return
	}

	writeJSON(w, http.StatusOK, pool)
}

func (r *Router) handleDeleteIPAMPool(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	poolID := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	if err := r.ipamService.DeletePool(req.Context(), poolID); err != nil {
		r.logger.Printf("ipam delete pool error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to delete pool")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- IPAM reservations ---

func (r *Router) handleListIPAMReservations(w http.ResponseWriter, req *http.Request) {
	if _, ok := r.authenticate(w, req); !ok {
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	// /api/v1/ipam/pools/{id}/reservations
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	poolID := path
	if idx := strings.Index(poolID, "/"); idx >= 0 {
		poolID = poolID[:idx]
	}

	reservations, err := r.ipamService.ListReservations(req.Context(), poolID)
	if err != nil {
		r.logger.Printf("ipam list reservations error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list reservations")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"reservations": reservations})
}

func (r *Router) handleAllocateIPAMAddress(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" && claims.Role != "operator" {
		writeError(w, http.StatusForbidden, "forbidden", "operator or admin role required")
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	// /api/v1/ipam/pools/{id}/allocate
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/pools/")
	poolID := path
	if idx := strings.Index(poolID, "/"); idx >= 0 {
		poolID = poolID[:idx]
	}

	var body struct {
		PeerID string `json:"peer_id"`
		Label  string `json:"label"`
	}
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	resID := generateID("res")
	res, err := r.ipamService.AllocateAddress(req.Context(), resID, poolID, body.PeerID, body.Label)
	if err != nil {
		r.logger.Printf("ipam allocate error: %v", err)
		writeError(w, http.StatusBadRequest, "allocate_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (r *Router) handleReleaseIPAMReservation(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" && claims.Role != "operator" {
		writeError(w, http.StatusForbidden, "forbidden", "operator or admin role required")
		return
	}

	if r.ipamService == nil {
		writeError(w, http.StatusServiceUnavailable, "ipam_unavailable", "IPAM service not configured")
		return
	}

	// /api/v1/ipam/reservations/{id}
	resID := strings.TrimPrefix(req.URL.Path, "/api/v1/ipam/reservations/")
	if err := r.ipamService.ReleaseReservation(req.Context(), resID); err != nil {
		r.logger.Printf("ipam release error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to release reservation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func generateID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}
