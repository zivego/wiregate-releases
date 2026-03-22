package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type exportLine struct {
	ID           string         `json:"id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Result       string         `json:"result"`
	CreatedAt    string         `json:"created_at"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	HashMeta     struct {
		PrevHash  string `json:"prev_hash,omitempty"`
		EventHash string `json:"event_hash,omitempty"`
	} `json:"hash_meta,omitempty"`
}

// ExportNDJSON streams audit events as newline-delimited JSON and returns (count, bytes).
func (s *Service) ExportNDJSON(ctx context.Context, writer io.Writer, filter ListFilter) (int, int64, error) {
	if s == nil {
		return 0, 0, fmt.Errorf("audit service is not configured")
	}
	if writer == nil {
		return 0, 0, fmt.Errorf("writer is required")
	}

	cursor := ""
	count := 0
	var bytesWritten int64
	pageSize := 200

	for {
		page, err := s.ListPage(ctx, ListFilter{
			Action:       filter.Action,
			ResourceType: filter.ResourceType,
			Result:       filter.Result,
			ActorUserID:  filter.ActorUserID,
			Limit:        pageSize,
			Cursor:       cursor,
		})
		if err != nil {
			return count, bytesWritten, err
		}

		for _, event := range page.Events {
			line := exportLine{
				ID:           event.ID,
				ActorUserID:  event.ActorUserID,
				Action:       event.Action,
				ResourceType: event.ResourceType,
				ResourceID:   event.ResourceID,
				Result:       event.Result,
				CreatedAt:    event.CreatedAt.UTC().Format(timeLayoutExport),
				Metadata:     event.Metadata,
			}
			line.HashMeta.PrevHash = event.PrevHash
			line.HashMeta.EventHash = event.EventHash

			payload, err := json.Marshal(line)
			if err != nil {
				return count, bytesWritten, fmt.Errorf("audit export marshal: %w", err)
			}
			n, err := writer.Write(append(payload, '\n'))
			if err != nil {
				return count, bytesWritten, fmt.Errorf("audit export write: %w", err)
			}
			bytesWritten += int64(n)
			count++
		}

		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return count, bytesWritten, nil
}

const timeLayoutExport = "2006-01-02T15:04:05.999999999Z07:00"
