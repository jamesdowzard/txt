package web

import "sync"

const (
	EventTypeConversations   = "conversations"
	EventTypeDrafts          = "drafts"
	EventTypeHeartbeat       = "heartbeat"
	EventTypeMessages        = "messages"
	EventTypeStatus          = "status"
	EventTypeTyping          = "typing"
	EventTypeIncomingMessage = "incoming_message"

	subscriberChannelBuffer = 8
	subscriberMaxPending    = 128
)

// StreamEvent is a small invalidation payload pushed to connected browsers.
// When Type == EventTypeIncomingMessage, Body + MessageID + ConversationName
// carry enough context for the native Tauri shell (see desktop/src-tauri/src/
// notifications.rs) to show a system notification with an inline reply action
// without round-tripping.
type StreamEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id,omitempty"`
	Connected      *bool  `json:"connected,omitempty"`
	SenderName     string `json:"sender_name,omitempty"`
	SenderNumber   string `json:"sender_number,omitempty"`
	Timestamp      int64  `json:"timestamp,omitempty"`
	Typing         *bool  `json:"typing,omitempty"`

	// Populated only for EventTypeIncomingMessage.
	MessageID        string `json:"message_id,omitempty"`
	Body             string `json:"body,omitempty"`
	IsGroup          bool   `json:"is_group,omitempty"`
	NotificationMode string `json:"notification_mode,omitempty"`
	ConversationName string `json:"conversation_name,omitempty"`
}

type eventSubscriber struct {
	out    chan StreamEvent
	notify chan struct{}
	done   chan struct{}

	mu      sync.Mutex
	order   []string
	pending map[string]StreamEvent
}

func newEventSubscriber() *eventSubscriber {
	sub := &eventSubscriber{
		out:     make(chan StreamEvent, subscriberChannelBuffer),
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
		pending: make(map[string]StreamEvent),
	}
	go sub.run()
	return sub
}

func (s *eventSubscriber) run() {
	for {
		select {
		case <-s.notify:
			for {
				evt, ok := s.next()
				if !ok {
					break
				}
				select {
				case s.out <- evt:
				case <-s.done:
					return
				}
			}
		case <-s.done:
			return
		}
	}
}

func (s *eventSubscriber) enqueue(evt StreamEvent) {
	key := streamEventKey(evt)

	s.mu.Lock()
	if _, exists := s.pending[key]; !exists {
		s.order = append(s.order, key)
	}
	s.pending[key] = evt
	if len(s.order) > subscriberMaxPending {
		s.collapseLocked(evt)
	}
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *eventSubscriber) next() (StreamEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.order) == 0 {
		return StreamEvent{}, false
	}
	key := s.order[0]
	s.order = s.order[1:]
	evt := s.pending[key]
	delete(s.pending, key)
	return evt, true
}

func (s *eventSubscriber) collapseLocked(incoming StreamEvent) {
	statusEvt, hasStatus := s.pending[EventTypeStatus]
	if incoming.Type == EventTypeStatus {
		statusEvt = incoming
		hasStatus = true
	}

	s.order = s.order[:0]
	s.pending = make(map[string]StreamEvent)
	if hasStatus {
		s.pending[EventTypeStatus] = statusEvt
		s.order = append(s.order, EventTypeStatus)
	}
	s.pending[EventTypeConversations] = StreamEvent{Type: EventTypeConversations}
	s.pending[streamEventKey(StreamEvent{Type: EventTypeMessages})] = StreamEvent{Type: EventTypeMessages}
	s.pending[streamEventKey(StreamEvent{Type: EventTypeDrafts})] = StreamEvent{Type: EventTypeDrafts}
	s.order = append(s.order, EventTypeConversations, streamEventKey(StreamEvent{Type: EventTypeMessages}), streamEventKey(StreamEvent{Type: EventTypeDrafts}))
}

func (s *eventSubscriber) close() {
	close(s.done)
}

func streamEventKey(evt StreamEvent) string {
	switch evt.Type {
	case EventTypeMessages, EventTypeDrafts, EventTypeTyping:
		return evt.Type + ":" + evt.ConversationID
	case EventTypeIncomingMessage:
		// Keyed by message ID so each new-message event stays discrete —
		// collapsing them would drop notifications the shell still needs.
		return evt.Type + ":" + evt.MessageID
	default:
		return evt.Type
	}
}

// EventBroker fans out lightweight invalidation events to SSE subscribers.
type EventBroker struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]*eventSubscriber
}

func NewEventBroker() *EventBroker {
	return &EventBroker{
		subscribers: make(map[int]*eventSubscriber),
	}
}

func (b *EventBroker) Subscribe() (int, <-chan StreamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	sub := newEventSubscriber()
	b.subscribers[id] = sub
	return id, sub.out
}

func (b *EventBroker) Unsubscribe(id int) {
	b.mu.Lock()
	sub := b.subscribers[id]
	delete(b.subscribers, id)
	b.mu.Unlock()
	if sub != nil {
		sub.close()
	}
}

func (b *EventBroker) Publish(evt StreamEvent) {
	b.mu.RLock()
	subs := make([]*eventSubscriber, 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		subs = append(subs, sub)
	}
	b.mu.RUnlock()

	for _, sub := range subs {
		sub.enqueue(evt)
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

func (b *EventBroker) PublishTyping(conversationID, senderName, senderNumber string, typing bool) {
	if b == nil {
		return
	}
	b.Publish(StreamEvent{
		Type:           EventTypeTyping,
		ConversationID: conversationID,
		SenderName:     senderName,
		SenderNumber:   senderNumber,
		Typing:         &typing,
	})
}

// PublishIncomingMessage fans an incoming-message notification out to SSE
// subscribers. Unlike EventTypeMessages (a coarse "refresh the thread" nudge),
// this event carries the full payload the native Tauri shell needs to show a
// system notification with an inline reply action — no callback round-trips.
//
// Callers are responsible for filtering out IsFromMe and old messages before
// publishing; the shell still re-checks NotificationMode / IsMuted so backend
// and shell can't accidentally double-notify.
func (b *EventBroker) PublishIncomingMessage(evt StreamEvent) {
	if b == nil {
		return
	}
	evt.Type = EventTypeIncomingMessage
	b.Publish(evt)
}
