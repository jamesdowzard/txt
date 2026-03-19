package web

import "sync"

const (
	EventTypeConversations = "conversations"
	EventTypeDrafts        = "drafts"
	EventTypeMessages      = "messages"
	EventTypeStatus        = "status"
)

// StreamEvent is a small invalidation payload pushed to connected browsers.
type StreamEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
	Connected      *bool  `json:"connected,omitempty"`
}

// EventBroker fans out lightweight invalidation events to SSE subscribers.
type EventBroker struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan StreamEvent
}

func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: make(map[int]chan StreamEvent),
	}
}

func (b *EventBroker) Subscribe() (int, <-chan StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan StreamEvent, 32)
	b.subscribers[id] = ch
	return id, ch
}

func (b *EventBroker) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, id)
}

func (b *EventBroker) Publish(evt StreamEvent) {
	b.mu.RLock()
	subs := make([]chan StreamEvent, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- evt:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

func (b *EventBroker) PublishConversations() {
	if b == nil {
		return
	}
	b.Publish(StreamEvent{Type: EventTypeConversations})
}

func (b *EventBroker) PublishDrafts(conversationID string) {
	if b == nil {
		return
	}
	b.Publish(StreamEvent{
		Type:           EventTypeDrafts,
		ConversationID: conversationID,
	})
}

func (b *EventBroker) PublishMessages(conversationID string) {
	if b == nil {
		return
	}
	b.Publish(StreamEvent{
		Type:           EventTypeMessages,
		ConversationID: conversationID,
	})
}

func (b *EventBroker) PublishStatus(connected bool) {
	if b == nil {
		return
	}
	b.Publish(StreamEvent{
		Type:      EventTypeStatus,
		Connected: &connected,
	})
}
