// Package events provides an in-memory pub-sub broker for real-time SSE streaming.
package events

import (
	"context"
	"sync"
	"time"
)

// Event represents a real-time event published to SSE subscribers.
type Event struct {
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	Result       string         `json:"result,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
}

// Broker manages SSE subscriber connections and fans out events.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan Event
	nextID      uint64
}

// NewBroker creates a new event broker.
func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[uint64]chan Event),
	}
}

// Subscribe registers a new SSE client. The returned channel receives events
// until the context is cancelled or the broker is closed. bufSize controls
// backpressure — slow consumers will have events dropped when the buffer is full.
func (b *Broker) Subscribe(ctx context.Context, bufSize int) <-chan Event {
	if bufSize <= 0 {
		bufSize = 64
	}
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subscribers[id] = ch
	b.mu.Unlock()

	// Auto-unsubscribe when the client disconnects.
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subscribers, id)
		close(ch)
		b.mu.Unlock()
	}()

	return ch
}

// Publish fans out an event to all connected subscribers.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that subscriber.
func (b *Broker) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Subscriber cannot keep up — drop event.
		}
	}
}

// SubscriberCount returns the number of active SSE connections.
func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
