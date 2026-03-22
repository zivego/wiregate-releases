// Package notification provides email notifications for security-critical events.
package notification

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zivego/wiregate/internal/events"
)

// UserProvider resolves admin user emails for notification delivery.
type UserProvider interface {
	ListAdminEmails(ctx context.Context) ([]string, error)
	GetEmailByUserID(ctx context.Context, userID string) (string, error)
}

// PreferenceStore reads per-user notification preferences.
type PreferenceStore interface {
	IsEnabled(ctx context.Context, userID string, channel string) (bool, error)
	SetEnabled(ctx context.Context, userID string, channel string, enabled bool) error
	ListEnabledUsers(ctx context.Context, channel string) ([]string, error)
}

// Sender delivers a rendered message.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Config holds notification system settings.
type Config struct {
	Enabled bool
	Sender  Sender
}

// Service listens to the event broker and dispatches email notifications
// for security-critical events (approval requests, approvals, rejections).
type Service struct {
	cfg    Config
	users  UserProvider
	prefs  PreferenceStore
	logger *log.Logger
}

// NewService creates a notification service.
func NewService(cfg Config, users UserProvider, prefs PreferenceStore, logger *log.Logger) *Service {
	return &Service{cfg: cfg, users: users, prefs: prefs, logger: logger}
}

// notifiableActions lists event actions that trigger email notifications.
var notifiableActions = map[string]bool{
	"security.approval.request": true,
	"security.approval.approve": true,
	"security.approval.reject":  true,
	"agent.revoke":              true,
	"user.delete":               true,
	"user.create":               true,
}

// Start subscribes to the event broker and dispatches notifications in background.
// Blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context, broker *events.Broker) {
	if !s.cfg.Enabled || s.cfg.Sender == nil {
		s.logger.Printf("notifications disabled (enabled=%v, sender=%v)", s.cfg.Enabled, s.cfg.Sender != nil)
		return
	}

	ch := broker.Subscribe(ctx, 128)
	s.logger.Printf("notification service started, listening for events")

	for {
		select {
		case <-ctx.Done():
			s.logger.Printf("notification service stopped")
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if !notifiableActions[event.Action] {
				continue
			}
			s.handleEvent(ctx, event)
		}
	}
}

func (s *Service) handleEvent(ctx context.Context, event events.Event) {
	subject, body := renderNotification(event)
	if subject == "" {
		return
	}

	recipients, err := s.resolveRecipients(ctx, event)
	if err != nil {
		s.logger.Printf("notification: resolve recipients: %v", err)
		return
	}

	for _, email := range recipients {
		sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := s.cfg.Sender.Send(sendCtx, email, subject, body); err != nil {
			s.logger.Printf("notification: send to %s: %v", email, err)
		}
		cancel()
	}
}

func (s *Service) resolveRecipients(ctx context.Context, event events.Event) ([]string, error) {
	// For approval requests, notify all admins except the requester.
	if event.Action == "security.approval.request" {
		emails, err := s.users.ListAdminEmails(ctx)
		if err != nil {
			return nil, fmt.Errorf("list admin emails: %w", err)
		}

		actorEmail := ""
		if event.ActorUserID != "" {
			actorEmail, _ = s.users.GetEmailByUserID(ctx, event.ActorUserID)
		}

		var filtered []string
		for _, e := range emails {
			if e != actorEmail {
				filtered = append(filtered, e)
			}
		}
		return s.filterByPreferences(ctx, filtered)
	}

	// For approval decisions and critical actions, notify all admins.
	emails, err := s.users.ListAdminEmails(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin emails: %w", err)
	}
	return s.filterByPreferences(ctx, emails)
}

func (s *Service) filterByPreferences(ctx context.Context, emails []string) ([]string, error) {
	if s.prefs == nil {
		return emails, nil
	}
	enabledUsers, err := s.prefs.ListEnabledUsers(ctx, "email")
	if err != nil {
		// Fail-open: send to all if preference check fails.
		s.logger.Printf("notification: preference check failed, sending to all: %v", err)
		return emails, nil
	}
	enabledSet := make(map[string]bool, len(enabledUsers))
	for _, uid := range enabledUsers {
		enabledSet[uid] = true
	}
	// If no preferences stored at all, default to all enabled.
	if len(enabledSet) == 0 {
		return emails, nil
	}
	var filtered []string
	for _, e := range emails {
		filtered = append(filtered, e)
	}
	return filtered, nil
}

func renderNotification(event events.Event) (subject, body string) {
	actor := event.ActorUserID
	if actor == "" {
		actor = "system"
	}
	resource := event.ResourceType
	if event.ResourceID != "" {
		resource += " " + event.ResourceID
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("WireGate Event: %s\n\n", event.Action))
	sb.WriteString(fmt.Sprintf("Action: %s\n", event.Action))
	sb.WriteString(fmt.Sprintf("Resource: %s\n", resource))
	sb.WriteString(fmt.Sprintf("Actor: %s\n", actor))
	sb.WriteString(fmt.Sprintf("Result: %s\n", event.Result))
	sb.WriteString(fmt.Sprintf("Time: %s\n", event.Timestamp.Format(time.RFC3339)))

	if len(event.Metadata) > 0 {
		sb.WriteString("\nDetails:\n")
		for k, v := range event.Metadata {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	switch event.Action {
	case "security.approval.request":
		subject = fmt.Sprintf("[WireGate] Approval required: %s on %s", metaStr(event.Metadata, "action"), resource)
	case "security.approval.approve":
		subject = fmt.Sprintf("[WireGate] Approval granted: %s on %s", metaStr(event.Metadata, "action"), resource)
	case "security.approval.reject":
		subject = fmt.Sprintf("[WireGate] Approval rejected: %s on %s", metaStr(event.Metadata, "action"), resource)
	case "agent.revoke":
		subject = fmt.Sprintf("[WireGate] Agent revoked: %s", event.ResourceID)
	case "user.create":
		subject = fmt.Sprintf("[WireGate] New user created: %s", metaStr(event.Metadata, "email"))
	case "user.delete":
		subject = fmt.Sprintf("[WireGate] User deleted: %s", metaStr(event.Metadata, "email"))
	default:
		subject = fmt.Sprintf("[WireGate] %s", event.Action)
	}

	return subject, sb.String()
}

func metaStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
