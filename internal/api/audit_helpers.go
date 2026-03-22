package api

import (
	"context"
	"time"

	"github.com/zivego/wiregate/internal/audit"
	"github.com/zivego/wiregate/internal/events"
)

func (r *Router) recordAuditEvent(ctx context.Context, event audit.Event) {
	if r.auditService == nil {
		r.emitRuntimeLogFromAuditEvent(ctx, event)
		r.publishEvent(event)
		return
	}
	if err := r.auditService.Record(ctx, event); err != nil {
		r.logger.Printf("audit record error: %v", err)
		return
	}
	r.emitRuntimeLogFromAuditEvent(ctx, event)
	r.publishEvent(event)
}

func (r *Router) publishEvent(ae audit.Event) {
	if r.eventBroker == nil {
		return
	}
	r.eventBroker.Publish(events.Event{
		Action:       ae.Action,
		ResourceType: ae.ResourceType,
		ResourceID:   ae.ResourceID,
		ActorUserID:  ae.ActorUserID,
		Result:       ae.Result,
		Metadata:     ae.Metadata,
		Timestamp:    time.Now().UTC(),
	})
}
