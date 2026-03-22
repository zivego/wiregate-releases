package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	wirelogging "github.com/zivego/wiregate/internal/logging"
)

type logSinkResponse struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Type      string                  `json:"type"`
	Enabled   bool                    `json:"enabled"`
	Syslog    logSyslogConfigResponse `json:"syslog"`
	CreatedAt string                  `json:"created_at"`
	UpdatedAt string                  `json:"updated_at"`
}

type logSyslogConfigResponse struct {
	Transport        string `json:"transport"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Format           string `json:"format"`
	Facility         int    `json:"facility"`
	AppName          string `json:"app_name"`
	HostnameOverride string `json:"hostname_override,omitempty"`
	CACertFile       string `json:"ca_cert_file,omitempty"`
	ClientCertFile   string `json:"client_cert_file,omitempty"`
	ClientKeyFile    string `json:"client_key_file,omitempty"`
}

type logSinkRequest struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Enabled bool                   `json:"enabled"`
	Syslog  logSyslogConfigRequest `json:"syslog"`
}

type logSyslogConfigRequest struct {
	Transport        string `json:"transport"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Format           string `json:"format"`
	Facility         int    `json:"facility"`
	AppName          string `json:"app_name"`
	HostnameOverride string `json:"hostname_override"`
	CACertFile       string `json:"ca_cert_file"`
	ClientCertFile   string `json:"client_cert_file"`
	ClientKeyFile    string `json:"client_key_file"`
}

type logRouteRuleResponse struct {
	ID          string   `json:"id"`
	SinkID      string   `json:"sink_id"`
	Categories  []string `json:"categories"`
	MinSeverity string   `json:"min_severity"`
	Enabled     bool     `json:"enabled"`
}

type patchLogRoutesRequest struct {
	Routes []logRouteRuleRequest `json:"routes"`
}

type logRouteRuleRequest struct {
	ID          string   `json:"id"`
	SinkID      string   `json:"sink_id"`
	Categories  []string `json:"categories"`
	MinSeverity string   `json:"min_severity"`
	Enabled     bool     `json:"enabled"`
}

type logStatusSinkResponse struct {
	SinkID              string `json:"sink_id"`
	SinkName            string `json:"sink_name"`
	SinkType            string `json:"sink_type"`
	Enabled             bool   `json:"enabled"`
	QueueDepth          int    `json:"queue_depth"`
	DroppedEvents       int    `json:"dropped_events"`
	TotalDelivered      int    `json:"total_delivered"`
	TotalFailed         int    `json:"total_failed"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastAttemptedAt     string `json:"last_attempted_at,omitempty"`
	LastDeliveredAt     string `json:"last_delivered_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	UpdatedAt           string `json:"updated_at"`
	DeadLetterCount     int    `json:"dead_letter_count"`
}

type logFailureResponse struct {
	ID           string         `json:"id"`
	SinkID       string         `json:"sink_id"`
	SinkName     string         `json:"sink_name"`
	OccurredAt   string         `json:"occurred_at"`
	Category     string         `json:"category"`
	Severity     string         `json:"severity"`
	Message      string         `json:"message"`
	Action       string         `json:"action,omitempty"`
	ErrorMessage string         `json:"error_message"`
	TestDelivery bool           `json:"test_delivery"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type testDeliveryRequest struct {
	SinkID string `json:"sink_id"`
}

func (r *Router) handleListLoggingSinks(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canViewLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	sinks, err := r.loggingService.ListSinks(req.Context())
	if err != nil {
		r.logger.Printf("list logging sinks error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list log sinks")
		return
	}
	resp := make([]logSinkResponse, 0, len(sinks))
	for _, sink := range sinks {
		resp = append(resp, logSinkToResponse(sink))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sinks": resp})
}

func (r *Router) handleCreateLoggingSink(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "logging.sink.create") {
		return
	}
	var body logSinkRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	sink, err := r.loggingService.CreateSink(req.Context(), wirelogging.Sink{
		Name:    strings.TrimSpace(body.Name),
		Type:    strings.TrimSpace(body.Type),
		Enabled: body.Enabled,
		Syslog: wirelogging.SyslogConfig{
			Transport:        strings.TrimSpace(body.Syslog.Transport),
			Host:             strings.TrimSpace(body.Syslog.Host),
			Port:             body.Syslog.Port,
			Format:           strings.TrimSpace(body.Syslog.Format),
			Facility:         body.Syslog.Facility,
			AppName:          strings.TrimSpace(body.Syslog.AppName),
			HostnameOverride: strings.TrimSpace(body.Syslog.HostnameOverride),
			CACertFile:       strings.TrimSpace(body.Syslog.CACertFile),
			ClientCertFile:   strings.TrimSpace(body.Syslog.ClientCertFile),
			ClientKeyFile:    strings.TrimSpace(body.Syslog.ClientKeyFile),
		},
	})
	if err != nil {
		writeLoggingError(w, err)
		return
	}
	r.emitRuntimeLog(req.Context(), wirelogging.Event{
		Category: "system",
		Severity: "info",
		Message:  "log sink created",
		Action:   "logging.sink.create",
		Metadata: map[string]any{"sink_id": sink.ID, "sink_type": sink.Type, "enabled": sink.Enabled},
	})
	writeJSON(w, http.StatusCreated, logSinkToResponse(sink))
}

func (r *Router) handlePatchLoggingSink(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	sinkID, ok := parseLoggingSinkAction(req.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "log sink not found")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "logging.sink.update") {
		return
	}
	var body logSinkRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	sink, err := r.loggingService.UpdateSink(req.Context(), wirelogging.Sink{
		ID:      sinkID,
		Name:    strings.TrimSpace(body.Name),
		Type:    strings.TrimSpace(body.Type),
		Enabled: body.Enabled,
		Syslog: wirelogging.SyslogConfig{
			Transport:        strings.TrimSpace(body.Syslog.Transport),
			Host:             strings.TrimSpace(body.Syslog.Host),
			Port:             body.Syslog.Port,
			Format:           strings.TrimSpace(body.Syslog.Format),
			Facility:         body.Syslog.Facility,
			AppName:          strings.TrimSpace(body.Syslog.AppName),
			HostnameOverride: strings.TrimSpace(body.Syslog.HostnameOverride),
			CACertFile:       strings.TrimSpace(body.Syslog.CACertFile),
			ClientCertFile:   strings.TrimSpace(body.Syslog.ClientCertFile),
			ClientKeyFile:    strings.TrimSpace(body.Syslog.ClientKeyFile),
		},
	})
	if err != nil {
		writeLoggingError(w, err)
		return
	}
	r.emitRuntimeLog(req.Context(), wirelogging.Event{
		Category: "system",
		Severity: "info",
		Message:  "log sink updated",
		Action:   "logging.sink.update",
		Metadata: map[string]any{"sink_id": sink.ID, "enabled": sink.Enabled},
	})
	writeJSON(w, http.StatusOK, logSinkToResponse(sink))
}

func (r *Router) handleDeleteLoggingSink(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	sinkID, ok := parseLoggingSinkAction(req.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "log sink not found")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "logging.sink.delete") {
		return
	}
	if err := r.loggingService.DeleteSink(req.Context(), sinkID); err != nil {
		writeLoggingError(w, err)
		return
	}
	r.emitRuntimeLog(req.Context(), wirelogging.Event{
		Category: "system",
		Severity: "warn",
		Message:  "log sink deleted",
		Action:   "logging.sink.delete",
		Metadata: map[string]any{"sink_id": sinkID},
	})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) handleGetLoggingRoutes(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canViewLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	routes, err := r.loggingService.GetRoutes(req.Context())
	if err != nil {
		r.logger.Printf("get logging routes error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load logging routes")
		return
	}
	resp := make([]logRouteRuleResponse, 0, len(routes))
	for _, route := range routes {
		resp = append(resp, logRouteToResponse(route))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"routes":          resp,
		"redacted_fields": wirelogging.RedactedFields(),
	})
}

func (r *Router) handlePatchLoggingRoutes(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "logging.routes.update") {
		return
	}
	var body patchLogRoutesRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	routes := make([]wirelogging.RouteRule, 0, len(body.Routes))
	for _, route := range body.Routes {
		routes = append(routes, wirelogging.RouteRule{
			ID:          strings.TrimSpace(route.ID),
			SinkID:      strings.TrimSpace(route.SinkID),
			Categories:  route.Categories,
			MinSeverity: strings.TrimSpace(route.MinSeverity),
			Enabled:     route.Enabled,
		})
	}
	updated, err := r.loggingService.UpdateRoutes(req.Context(), routes)
	if err != nil {
		writeLoggingError(w, err)
		return
	}
	resp := make([]logRouteRuleResponse, 0, len(updated))
	for _, route := range updated {
		resp = append(resp, logRouteToResponse(route))
	}
	r.emitRuntimeLog(req.Context(), wirelogging.Event{
		Category: "system",
		Severity: "info",
		Message:  "logging routes updated",
		Action:   "logging.routes.update",
		Metadata: map[string]any{"route_count": len(updated)},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"routes":          resp,
		"redacted_fields": wirelogging.RedactedFields(),
	})
}

func (r *Router) handleGetLoggingStatus(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canViewLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	status, err := r.loggingService.GetStatus(req.Context())
	if err != nil {
		r.logger.Printf("get logging status error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load logging status")
		return
	}
	sinks, err := r.loggingService.ListSinks(req.Context())
	if err != nil {
		r.logger.Printf("list logging sinks for status error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load logging status")
		return
	}
	sinkByID := map[string]wirelogging.Sink{}
	for _, sink := range sinks {
		sinkByID[sink.ID] = sink
	}
	resp := make([]logStatusSinkResponse, 0, len(status.Sinks))
	for _, item := range status.Sinks {
		sink := sinkByID[item.SinkID]
		entry := logStatusSinkResponse{
			SinkID:              item.SinkID,
			SinkName:            sink.Name,
			SinkType:            sink.Type,
			Enabled:             sink.Enabled,
			QueueDepth:          item.QueueDepth,
			DroppedEvents:       item.DroppedEvents,
			TotalDelivered:      item.TotalDelivered,
			TotalFailed:         item.TotalFailed,
			ConsecutiveFailures: item.ConsecutiveFailures,
			LastError:           item.LastError,
			UpdatedAt:           item.UpdatedAt.Format(time.RFC3339Nano),
			DeadLetterCount:     status.DeadLetterCounts[item.SinkID],
		}
		if item.LastAttemptedAt != nil {
			entry.LastAttemptedAt = item.LastAttemptedAt.Format(time.RFC3339Nano)
		}
		if item.LastDeliveredAt != nil {
			entry.LastDeliveredAt = item.LastDeliveredAt.Format(time.RFC3339Nano)
		}
		resp = append(resp, entry)
	}
	failures := make([]logFailureResponse, 0, len(status.RecentFailures))
	for _, failure := range status.RecentFailures {
		failures = append(failures, logFailureResponse{
			ID:           failure.ID,
			SinkID:       failure.SinkID,
			SinkName:     sinkByID[failure.SinkID].Name,
			OccurredAt:   failure.OccurredAt.Format(time.RFC3339Nano),
			Category:     failure.Category,
			Severity:     failure.Severity,
			Message:      failure.Message,
			Action:       failure.Action,
			ErrorMessage: failure.ErrorMessage,
			TestDelivery: failure.TestDelivery,
			Metadata:     failure.Metadata,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queue_capacity":  status.QueueCapacity,
		"current_queued":  status.CurrentQueued,
		"redacted_fields": status.RedactedFields,
		"sinks":           resp,
		"recent_failures": failures,
	})
}

func (r *Router) handleTestLoggingDelivery(w http.ResponseWriter, req *http.Request) {
	if r.loggingService == nil {
		writeError(w, http.StatusNotFound, "not_found", "logging is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageLogging(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "logging.test_delivery") {
		return
	}
	var body testDeliveryRequest
	if req.ContentLength != 0 {
		if err := r.decodeJSONBody(w, req, &body); err != nil {
			writeDecodeError(w, err)
			return
		}
	}
	if err := r.loggingService.TestDelivery(req.Context(), strings.TrimSpace(body.SinkID)); err != nil {
		writeLoggingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": true,
		"sink_id":  strings.TrimSpace(body.SinkID),
	})
}

func logSinkToResponse(sink wirelogging.Sink) logSinkResponse {
	return logSinkResponse{
		ID:      sink.ID,
		Name:    sink.Name,
		Type:    sink.Type,
		Enabled: sink.Enabled,
		Syslog: logSyslogConfigResponse{
			Transport:        sink.Syslog.Transport,
			Host:             sink.Syslog.Host,
			Port:             sink.Syslog.Port,
			Format:           sink.Syslog.Format,
			Facility:         sink.Syslog.Facility,
			AppName:          sink.Syslog.AppName,
			HostnameOverride: sink.Syslog.HostnameOverride,
			CACertFile:       sink.Syslog.CACertFile,
			ClientCertFile:   sink.Syslog.ClientCertFile,
			ClientKeyFile:    sink.Syslog.ClientKeyFile,
		},
		CreatedAt: sink.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: sink.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func logRouteToResponse(route wirelogging.RouteRule) logRouteRuleResponse {
	return logRouteRuleResponse{
		ID:          route.ID,
		SinkID:      route.SinkID,
		Categories:  route.Categories,
		MinSeverity: route.MinSeverity,
		Enabled:     route.Enabled,
	}
}

func writeLoggingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wirelogging.ErrSinkNotFound):
		writeError(w, http.StatusNotFound, "not_found", "log sink not found")
	case errors.Is(err, wirelogging.ErrInvalidSinkType), errors.Is(err, wirelogging.ErrInvalidTransport), errors.Is(err, wirelogging.ErrInvalidFormat), errors.Is(err, wirelogging.ErrInvalidRouteRule):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "logging operation failed")
	}
}

func canViewLogging(role string) bool {
	return role == "admin" || role == "operator"
}

func canManageLogging(role string) bool {
	return role == "admin"
}

func parseLoggingSinkAction(path string) (sinkID string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/v1/logging/sinks/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
}
