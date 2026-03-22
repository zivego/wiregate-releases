package audit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/zivego/wiregate/internal/persistence/auditrepo"
)

var ErrInvalidCursor = fmt.Errorf("invalid audit cursor")

// Event represents one immutable audit record.
type Event struct {
	ID           string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	CreatedAt    time.Time
	Metadata     map[string]any
	PrevHash     string
	EventHash    string
}

// ListFilter limits audit event results.
type ListFilter struct {
	Action       string
	ResourceType string
	Result       string
	ActorUserID  string
	Limit        int
	Cursor       string
}

type CursorPage struct {
	Events     []Event
	NextCursor string
}

// Service records and lists audit events.
type Service struct {
	repo *auditrepo.Repo
	mu   sync.Mutex
}

func NewService(repo *auditrepo.Repo) *Service {
	return &Service{repo: repo}
}

// Record writes an immutable audit event. When the repo is unset, it becomes a no-op.
func (s *Service) Record(ctx context.Context, event Event) error {
	if s == nil || s.repo == nil {
		return nil
	}

	id, err := newEventID()
	if err != nil {
		return fmt.Errorf("audit generate id: %w", err)
	}
	createdAt := time.Now().UTC()

	var metadataJSON string
	if len(event.Metadata) > 0 {
		payload, err := json.Marshal(event.Metadata)
		if err != nil {
			return fmt.Errorf("audit marshal metadata: %w", err)
		}
		metadataJSON = string(payload)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prevHash, err := s.repo.LatestHash(ctx)
	if err != nil {
		return fmt.Errorf("audit latest hash: %w", err)
	}
	eventHash, err := computeEventHash(id, event.ActorUserID, event.Action, event.ResourceType, event.ResourceID, event.Result, createdAt, metadataJSON, prevHash)
	if err != nil {
		return fmt.Errorf("audit compute event hash: %w", err)
	}

	record := auditrepo.Event{
		ID:           id,
		ActorUserID:  event.ActorUserID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Result:       event.Result,
		CreatedAt:    createdAt,
		MetadataJSON: metadataJSON,
		PrevHash:     prevHash,
		EventHash:    eventHash,
	}
	if err := s.repo.Insert(ctx, record); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}
	return nil
}

// List returns audit events ordered from newest to oldest.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Event, error) {
	page, err := s.ListPage(ctx, filter)
	if err != nil {
		return nil, err
	}
	return page.Events, nil
}

func (s *Service) ListPage(ctx context.Context, filter ListFilter) (CursorPage, error) {
	if s == nil || s.repo == nil {
		return CursorPage{}, nil
	}

	cursorTime, cursorID, err := decodeCursor(filter.Cursor)
	if err != nil {
		return CursorPage{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	records, hasMore, err := s.repo.ListPage(ctx, auditrepo.ListFilter{
		Action:       filter.Action,
		ResourceType: filter.ResourceType,
		Result:       filter.Result,
		ActorUserID:  filter.ActorUserID,
		Limit:        filter.Limit,
		CursorID:     cursorID,
		CursorTime:   cursorTime,
	})
	if err != nil {
		return CursorPage{}, fmt.Errorf("audit list: %w", err)
	}

	events := make([]Event, 0, len(records))
	for _, record := range records {
		event := Event{
			ID:           record.ID,
			ActorUserID:  record.ActorUserID,
			Action:       record.Action,
			ResourceType: record.ResourceType,
			ResourceID:   record.ResourceID,
			Result:       record.Result,
			CreatedAt:    record.CreatedAt,
			PrevHash:     record.PrevHash,
			EventHash:    record.EventHash,
		}
		if record.MetadataJSON != "" {
			if err := json.Unmarshal([]byte(record.MetadataJSON), &event.Metadata); err != nil {
				return CursorPage{}, fmt.Errorf("audit decode metadata: %w", err)
			}
		}
		events = append(events, event)
	}

	page := CursorPage{Events: events}
	if hasMore && len(events) > 0 {
		page.NextCursor = encodeCursor(events[len(events)-1].CreatedAt, events[len(events)-1].ID)
	}
	return page, nil
}

func computeEventHash(id, actorUserID, action, resourceType, resourceID, result string, createdAt time.Time, metadataJSON, prevHash string) (string, error) {
	payload, err := json.Marshal(struct {
		ID           string `json:"id"`
		ActorUserID  string `json:"actor_user_id,omitempty"`
		Action       string `json:"action"`
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id,omitempty"`
		Result       string `json:"result"`
		CreatedAt    string `json:"created_at"`
		MetadataJSON string `json:"metadata_json,omitempty"`
	}{
		ID:           id,
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Result:       result,
		CreatedAt:    createdAt.Format(time.RFC3339Nano),
		MetadataJSON: metadataJSON,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(prevHash), payload...))
	return hex.EncodeToString(sum[:]), nil
}

type auditCursor struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeCursor(createdAt time.Time, id string) string {
	payload, _ := json.Marshal(auditCursor{
		CreatedAt: createdAt.UTC().Format(time.RFC3339Nano),
		ID:        id,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(value string) (time.Time, string, error) {
	if value == "" {
		return time.Time{}, "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", err
	}
	var cursor auditCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return time.Time{}, "", err
	}
	if cursor.CreatedAt == "" || cursor.ID == "" {
		return time.Time{}, "", fmt.Errorf("cursor is incomplete")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return time.Time{}, "", err
	}
	return createdAt, cursor.ID, nil
}
