package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const sseHeartbeatInterval = 30 * time.Second

// handleSSE streams real-time events to authenticated clients via Server-Sent Events.
// GET /api/v1/events
func (r *Router) handleSSE(w http.ResponseWriter, req *http.Request) {
	if r.eventBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "not_available", "event stream not configured")
		return
	}

	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	_ = claims // all authenticated users can subscribe

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := req.Context()
	events := r.eventBroker.Subscribe(ctx, 64)

	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Action, data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
