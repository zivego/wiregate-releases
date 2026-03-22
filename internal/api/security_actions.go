package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/enrollment"
	"github.com/zivego/wiregate/internal/security"
	"github.com/zivego/wiregate/pkg/wgadapter"
)

func (r *Router) executeApprovedSecurityAction(ctx context.Context, approvalID, approvedByUserID string) (any, security.Approval, error) {
	approval, err := r.securityService.GetApproval(ctx, approvalID)
	if err != nil {
		return nil, security.Approval{}, err
	}
	if approval.Status != "pending" {
		return nil, security.Approval{}, security.ErrApprovalStateConflict
	}
	if approval.RequestedByUserID == approvedByUserID {
		return nil, security.Approval{}, security.ErrApprovalSelfDecision
	}

	var result any
	switch approval.Action {
	case "agent.revoke":
		result, err = r.executeApprovedAgentRevoke(ctx, approval.ResourceID, approvedByUserID, approval.ID)
	case "agent.reissue":
		result, err = r.executeApprovedAgentReissue(ctx, approval.ResourceID, approvedByUserID, approval.ID)
	case "agent.rotate":
		result, err = r.executeApprovedAgentRotate(ctx, approval.ResourceID, approvedByUserID, approval.ID)
	default:
		return nil, security.Approval{}, fmt.Errorf("unsupported approval action: %s", approval.Action)
	}
	if err != nil {
		return nil, security.Approval{}, err
	}

	approved, err := r.securityService.MarkApproved(ctx, approvalID, approvedByUserID)
	if err != nil {
		return nil, security.Approval{}, err
	}
	return result, approved, nil
}

func (r *Router) executeApprovedAgentRevoke(ctx context.Context, agentID, actorUserID, approvalID string) (agentInventoryResponse, error) {
	updated, err := r.enrollmentService.ChangeAgentState(ctx, agentID, "revoke")
	if errors.Is(err, enrollment.ErrAgentNotFound) || errors.Is(err, enrollment.ErrAgentStateConflict) || errors.Is(err, enrollment.ErrInvalidEnrollmentRequest) {
		return agentInventoryResponse{}, err
	}
	if err != nil {
		return agentInventoryResponse{}, err
	}
	if updated.Peer != nil {
		if err := r.wgService.ApplyPeer(ctx, wgadapter.ApplyPeerInput{
			PeerID:     updated.Peer.ID,
			PublicKey:  updated.Peer.PublicKey,
			AllowedIPs: updated.Peer.AllowedIPs,
			Action:     "remove",
		}); err != nil {
			return agentInventoryResponse{}, err
		}
	}
	r.recordAuditEvent(ctx, audit.Event{
		ActorUserID:  actorUserID,
		Action:       "agent.revoke",
		ResourceType: "agent",
		ResourceID:   updated.ID,
		Result:       "success",
		Metadata: map[string]any{
			"agent_status": updated.Status,
			"peer_status":  peerStatusFromInventory(updated.Peer),
			"approval_id":  approvalID,
		},
	})
	return r.agentInventoryToResponse(ctx, updated)
}

func (r *Router) executeApprovedAgentReissue(ctx context.Context, agentID, actorUserID, approvalID string) (reissueAgentResponse, error) {
	token, rawToken, _, err := r.enrollmentService.ReissueAgentEnrollment(ctx, agentID, actorUserID)
	if err != nil {
		return reissueAgentResponse{}, err
	}
	r.recordAuditEvent(ctx, audit.Event{
		ActorUserID:  actorUserID,
		Action:       "agent.reissue",
		ResourceType: "agent",
		ResourceID:   agentID,
		Result:       "success",
		Metadata: map[string]any{
			"token_id":          token.ID,
			"bound_identity":    token.BoundIdentity,
			"access_policy_ids": token.AccessPolicyIDs,
			"approval_id":       approvalID,
		},
	})
	return reissueAgentResponse{
		ID:              token.ID,
		Model:           token.Model,
		Scope:           token.Scope,
		Status:          token.Status,
		BoundIdentity:   token.BoundIdentity,
		AccessPolicyIDs: token.AccessPolicyIDs,
		ExpiresAt:       token.ExpiresAt.Format(time.RFC3339),
		CreatedByUserID: token.CreatedByUserID,
		CreatedAt:       token.CreatedAt.Format(time.RFC3339),
		Token:           rawToken,
	}, nil
}

func (r *Router) executeApprovedAgentRotate(ctx context.Context, agentID, actorUserID, approvalID string) (agentInventoryResponse, error) {
	updated, err := r.enrollmentService.MarkAgentRotationPending(ctx, agentID)
	if err != nil {
		return agentInventoryResponse{}, err
	}
	r.recordAuditEvent(ctx, audit.Event{
		ActorUserID:  actorUserID,
		Action:       "agent.rotate.request",
		ResourceType: "agent",
		ResourceID:   updated.ID,
		Result:       "success",
		Metadata: map[string]any{
			"peer_id":     peerIDFromInventory(updated.Peer),
			"peer_status": peerStatusFromInventory(updated.Peer),
			"approval_id": approvalID,
		},
	})
	return r.agentInventoryToResponse(ctx, updated)
}
