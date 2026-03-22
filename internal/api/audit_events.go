package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/zivego/wiregate/internal/audit"
)

type auditEventResponse struct {
	ID           string         `json:"id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Result       string         `json:"result"`
	CreatedAt    string         `json:"created_at"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	HashMeta     *auditHashMeta `json:"hash_meta,omitempty"`
}

type auditHashMeta struct {
	PrevHash  string `json:"prev_hash,omitempty"`
	EventHash string `json:"event_hash,omitempty"`
}

// handleListAuditEvents handles GET /api/v1/audit-events for authenticated users.
func (r *Router) handleListAuditEvents(w http.ResponseWriter, req *http.Request) {
	_, ok := r.authenticate(w, req)
	if !ok {
		return
	}

	limit := 50
	pageSizeRaw := req.URL.Query().Get("page_size")
	if pageSizeRaw == "" {
		pageSizeRaw = req.URL.Query().Get("limit")
	}
	if raw := pageSizeRaw; raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "validation_failed", "page_size must be a positive integer")
			return
		}
		limit = n
	}

	page, err := r.auditService.ListPage(req.Context(), audit.ListFilter{
		Action:       req.URL.Query().Get("action"),
		ResourceType: req.URL.Query().Get("resource_type"),
		Result:       req.URL.Query().Get("result"),
		ActorUserID:  req.URL.Query().Get("actor_user_id"),
		Limit:        limit,
		Cursor:       req.URL.Query().Get("cursor"),
	})
	if err != nil && errors.Is(err, audit.ErrInvalidCursor) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid audit cursor")
		return
	}
	if err != nil {
		r.logger.Printf("list audit events error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list audit events")
		return
	}

	resp := make([]auditEventResponse, 0, len(page.Events))
	for _, event := range page.Events {
		resp = append(resp, auditEventResponse{
			ID:           event.ID,
			ActorUserID:  event.ActorUserID,
			Action:       event.Action,
			ResourceType: event.ResourceType,
			ResourceID:   event.ResourceID,
			Result:       event.Result,
			CreatedAt:    event.CreatedAt.Format(time.RFC3339Nano),
			Metadata:     event.Metadata,
			HashMeta: &auditHashMeta{
				PrevHash:  event.PrevHash,
				EventHash: event.EventHash,
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": resp, "next_cursor": page.NextCursor})
}
