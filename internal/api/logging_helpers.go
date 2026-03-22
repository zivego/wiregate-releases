package api

import (
	"context"
	"strings"

	"github.com/zivego/wiregate/internal/audit"
	wirelogging "github.com/zivego/wiregate/internal/logging"
)

func (r *Router) emitRuntimeLog(ctx context.Context, event wirelogging.Event) {
	if r.loggingService == nil {
		return
	}
	r.loggingService.Emit(ctx, event)
}

func (r *Router) emitRuntimeLogFromAuditEvent(ctx context.Context, event audit.Event) {
	if r.loggingService == nil {
		return
	}
	r.emitRuntimeLog(ctx, wirelogging.Event{
		Category:     auditEventCategory(event.Action),
		Severity:     auditEventSeverity(event.Result),
		Message:      auditEventMessage(event),
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Metadata:     event.Metadata,
	})
}

func auditEventCategory(action string) string {
	switch {
	case strings.HasPrefix(action, "auth."):
		return "auth"
	case strings.HasPrefix(action, "session."):
		return "session"
	case strings.HasPrefix(action, "user."):
		return "user_mgmt"
	case strings.HasPrefix(action, "policy.") || strings.HasPrefix(action, "access_policy."):
		return "policy"
	case strings.HasPrefix(action, "agent."):
		return "agent"
	case strings.HasPrefix(action, "enrollment"):
		return "enrollment"
	case strings.HasPrefix(action, "reconcile.") || strings.HasPrefix(action, "peer.reconcile"):
		return "reconcile"
	case strings.HasPrefix(action, "security."):
		return "security"
	default:
		return "system"
	}
}

func auditEventSeverity(result string) string {
	if strings.EqualFold(strings.TrimSpace(result), "failure") {
		return "warn"
	}
	return "info"
}

func auditEventMessage(event audit.Event) string {
	if action := strings.TrimSpace(event.Action); action != "" {
		return strings.ReplaceAll(action, ".", " ")
	}
	return "audit event"
}
