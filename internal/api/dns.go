package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	wiredns "github.com/zivego/wiregate/internal/dns"
)

type dnsConfigResponse struct {
	Enabled       bool     `json:"enabled"`
	Servers       []string `json:"servers"`
	SearchDomains []string `json:"search_domains"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

type patchDNSConfigRequest struct {
	Enabled       bool     `json:"enabled"`
	Servers       []string `json:"servers"`
	SearchDomains []string `json:"search_domains"`
}

func (r *Router) handleGetDNSConfig(w http.ResponseWriter, req *http.Request) {
	if r.dnsService == nil {
		writeError(w, http.StatusNotFound, "not_found", "dns configuration is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canViewDNS(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin or operator role required")
		return
	}
	config, err := r.dnsService.GetConfig(req.Context())
	if err != nil {
		r.logger.Printf("get dns config error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load dns config")
		return
	}
	writeJSON(w, http.StatusOK, dnsConfigToResponse(config))
}

func (r *Router) handlePatchDNSConfig(w http.ResponseWriter, req *http.Request) {
	if r.dnsService == nil {
		writeError(w, http.StatusNotFound, "not_found", "dns configuration is unavailable")
		return
	}
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if !canManageDNS(claims.Role) {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "dns.config.update") {
		return
	}
	var body patchDNSConfigRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	config, err := r.dnsService.UpdateConfig(req.Context(), wiredns.Config{
		Enabled:       body.Enabled,
		Servers:       normalizeStringList(body.Servers),
		SearchDomains: normalizeStringList(body.SearchDomains),
	})
	if err != nil {
		writeDNSError(w, err)
		return
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "dns.config.update",
		ResourceType: "dns_config",
		ResourceID:   "default",
		Result:       "success",
		Metadata: map[string]any{
			"enabled":        config.Enabled,
			"servers":        config.Servers,
			"search_domains": config.SearchDomains,
		},
	})
	writeJSON(w, http.StatusOK, dnsConfigToResponse(config))
}

func dnsConfigToResponse(config wiredns.Config) dnsConfigResponse {
	response := dnsConfigResponse{
		Enabled:       config.Enabled,
		Servers:       config.Servers,
		SearchDomains: config.SearchDomains,
	}
	if !config.UpdatedAt.IsZero() {
		response.UpdatedAt = config.UpdatedAt.Format(time.RFC3339Nano)
	}
	return response
}

func writeDNSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, wiredns.ErrInvalidConfig):
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "dns operation failed")
	}
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func canViewDNS(role string) bool {
	return role == "admin" || role == "operator"
}

func canManageDNS(role string) bool {
	return role == "admin"
}
