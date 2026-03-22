package api

import (
	"errors"
	"net/http"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/enrollment"
)

type performEnrollmentRequest struct {
	Token     string `json:"token"`
	Hostname  string `json:"hostname"`
	Platform  string `json:"platform"`
	PublicKey string `json:"public_key"`
}

type performEnrollmentResponse struct {
	Agent           agentInventoryResponse   `json:"agent"`
	AgentAuthToken  string                   `json:"agent_auth_token"`
	WireGuardConfig *wireGuardConfigResponse `json:"wireguard_config,omitempty"`
	Token           struct {
		ID            string `json:"id"`
		Model         string `json:"model"`
		Status        string `json:"status"`
		BoundIdentity string `json:"bound_identity,omitempty"`
	} `json:"token"`
}

func (r *Router) handlePerformEnrollment(w http.ResponseWriter, req *http.Request) {
	if !r.limitEnrollmentAttempt(w, req) {
		return
	}

	var body performEnrollmentRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}

	result, err := r.enrollmentService.PerformEnrollment(req.Context(), enrollment.PerformEnrollmentInput{
		Token:     body.Token,
		Hostname:  body.Hostname,
		Platform:  body.Platform,
		PublicKey: body.PublicKey,
	})

	if errors.Is(err, enrollment.ErrInvalidEnrollmentRequest) {
		r.recordEnrollmentFailure(req, result, body, "validation_failed")
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	if errors.Is(err, enrollment.ErrInvalidEnrollmentToken) {
		r.recordEnrollmentFailure(req, result, body, "invalid_token")
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid enrollment token")
		return
	}
	if errors.Is(err, enrollment.ErrTokenStateConflict) || errors.Is(err, enrollment.ErrEnrollmentConflict) {
		r.recordEnrollmentFailure(req, result, body, "conflict")
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		r.logger.Printf("perform enrollment error: %v", err)
		r.recordEnrollmentFailure(req, result, body, "internal_error")
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to perform enrollment")
		return
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		Action:       "enrollment.perform",
		ResourceType: "enrollment_token",
		ResourceID:   result.Token.ID,
		Result:       "success",
		Metadata: map[string]any{
			"agent_id":       result.Agent.ID,
			"hostname":       result.Agent.Hostname,
			"platform":       result.Agent.Platform,
			"public_key":     result.Agent.Peer.PublicKey,
			"token_model":    result.Token.Model,
			"bound_identity": result.Token.BoundIdentity,
			"token_status":   result.Token.Status,
		},
	})

	var resp performEnrollmentResponse
	respAgent, err := r.agentInventoryToResponse(req.Context(), result.Agent)
	if err != nil {
		r.logger.Printf("perform enrollment map traffic mode error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to perform enrollment")
		return
	}
	resp.Agent = respAgent
	resp.AgentAuthToken = result.AgentAuthToken
	resp.WireGuardConfig = wireGuardConfigToResponse(result.WireGuardConfig)
	resp.Token.ID = result.Token.ID
	resp.Token.Model = result.Token.Model
	resp.Token.Status = result.Token.Status
	resp.Token.BoundIdentity = result.Token.BoundIdentity
	writeJSON(w, http.StatusCreated, resp)
}

func (r *Router) recordEnrollmentFailure(req *http.Request, result enrollment.PerformEnrollmentResult, body performEnrollmentRequest, reason string) {
	metadata := map[string]any{
		"hostname":   body.Hostname,
		"platform":   body.Platform,
		"public_key": body.PublicKey,
		"reason":     reason,
	}
	if result.Token.ID != "" {
		metadata["token_model"] = result.Token.Model
		metadata["token_status"] = result.Token.Status
		metadata["bound_identity"] = result.Token.BoundIdentity
	}
	r.recordAuditEvent(req.Context(), audit.Event{
		Action:       "enrollment.perform",
		ResourceType: "enrollment_token",
		ResourceID:   result.Token.ID,
		Result:       "failure",
		Metadata:     metadata,
	})
}
