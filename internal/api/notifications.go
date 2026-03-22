package api

import (
	"net/http"
)

type notificationPreferenceResponse struct {
	Channel string `json:"channel"`
	Enabled bool   `json:"enabled"`
}

type patchNotificationPreferenceRequest struct {
	Enabled bool `json:"enabled"`
}

func (r *Router) handleGetNotificationPreferences(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if r.notificationPrefs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"preferences": []notificationPreferenceResponse{{Channel: "email", Enabled: true}}})
		return
	}
	enabled, err := r.notificationPrefs.IsEnabled(req.Context(), claims.UserID, "email")
	if err != nil {
		r.logger.Printf("get notification preferences: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load preferences")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"preferences": []notificationPreferenceResponse{{Channel: "email", Enabled: enabled}},
	})
}

func (r *Router) handlePatchNotificationPreferences(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if r.notificationPrefs == nil {
		writeError(w, http.StatusNotFound, "not_found", "notification preferences are unavailable")
		return
	}

	var body patchNotificationPreferenceRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	if err := r.notificationPrefs.SetEnabled(req.Context(), claims.UserID, "email", body.Enabled); err != nil {
		r.logger.Printf("update notification preferences: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update preferences")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"preferences": []notificationPreferenceResponse{{Channel: "email", Enabled: body.Enabled}},
	})
}

