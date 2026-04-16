package db

import (
	"testing"
	"time"
)

func TestOutboxLifecycle(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().Unix()

	// Create
	item := &OutboxItem{
		ConversationID: "c1",
		Body:           "hi later",
		SendAt:         now + 60,
	}
	id, err := store.CreateOutboxItem(item)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
	if item.Status != OutboxStatusPending {
		t.Fatalf("default status = %q, want pending", item.Status)
	}

	// Validation
	if _, err := store.CreateOutboxItem(&OutboxItem{Body: "x", SendAt: now + 60}); err == nil {
		t.Fatal("expected error for missing conversation_id")
	}
	if _, err := store.CreateOutboxItem(&OutboxItem{ConversationID: "c1", Body: "x", SendAt: 0}); err == nil {
		t.Fatal("expected error for zero send_at")
	}

	// List pending
	items, err := store.ListOutboxItems(OutboxStatusPending, 100)
	if err != nil || len(items) != 1 {
		t.Fatalf("list pending: %v len=%d", err, len(items))
	}

	// Not yet due
	due, err := store.ListDueOutboxItems()
	if err != nil || len(due) != 0 {
		t.Fatalf("expected 0 due, got %d err=%v", len(due), err)
	}

	// Backdate to make due
	if _, err := store.db.Exec(`UPDATE outbox SET send_at = ? WHERE id = ?`, now-1, id); err != nil {
		t.Fatal(err)
	}
	due, _ = store.ListDueOutboxItems()
	if len(due) != 1 {
		t.Fatalf("expected 1 due, got %d", len(due))
	}

	// Mark sent
	if err := store.MarkOutboxSent(id, "msg-123"); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	items, _ = store.ListOutboxItems(OutboxStatusSent, 100)
	if len(items) != 1 || items[0].SentMessageID != "msg-123" {
		t.Fatalf("sent items = %+v", items)
	}

	// Delete
	if err := store.DeleteOutboxItem(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	items, _ = store.ListOutboxItems("", 100)
	if len(items) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(items))
	}
}

func TestOutboxRetryThenFail(t *testing.T) {
	store := newTestStore(t)
	id, _ := store.CreateOutboxItem(&OutboxItem{ConversationID: "c1", Body: "x", SendAt: time.Now().Unix() + 60})

	// Increment 3 times → still pending, attempts=3
	for i := 0; i < 3; i++ {
		if err := store.IncrementOutboxAttempts(id, "transient"); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := store.ListOutboxItems("pending", 100)
	if items[0].Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", items[0].Attempts)
	}

	// Fail
	if err := store.MarkOutboxFailed(id, "fatal"); err != nil {
		t.Fatal(err)
	}
	items, _ = store.ListOutboxItems("failed", 100)
	if len(items) != 1 || items[0].Error != "fatal" {
		t.Fatalf("failed items = %+v", items)
	}
}
