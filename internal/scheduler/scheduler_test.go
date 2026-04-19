package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/jamesdowzard/txt/internal/db"
)

// frozenClock returns a fixed time, mutatable by tests. Avoids time.Now()
// flakiness — the scheduler's "is this row due?" pivots on cfg.Now.
type frozenClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *frozenClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *frozenClock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestTick_FiresDueAndSkipsFuture verifies the scheduler's core fire-condition:
// pending + scheduled_at <= now fires; pending + scheduled_at > now doesn't.
func TestTick_FiresDueAndSkipsFuture(t *testing.T) {
	store := newTestStore(t)
	clock := &frozenClock{t: time.Unix(1_000_000, 0)}

	dueItem, err := store.CreateScheduledMessage(&db.ScheduledMessage{
		ConversationID: "conv-due",
		Body:           "past!",
		ScheduledAt:    clock.now().Add(-1 * time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("create due: %v", err)
	}
	futureItem, err := store.CreateScheduledMessage(&db.ScheduledMessage{
		ConversationID: "conv-future",
		Body:           "later",
		ScheduledAt:    clock.now().Add(10 * time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("create future: %v", err)
	}

	var sent []string
	sender := SenderFunc(func(convID, body, replyToID string) (string, error) {
		sent = append(sent, convID)
		return "msg-" + convID, nil
	})

	Tick(context.Background(), store, sender, Config{Now: clock.now, Logger: zerolog.Nop()})

	if len(sent) != 1 || sent[0] != "conv-due" {
		t.Fatalf("sent = %v, want [conv-due]", sent)
	}

	got, err := store.GetScheduledMessage(dueItem.ID)
	if err != nil || got == nil {
		t.Fatalf("get due: %v %v", got, err)
	}
	if got.Status != db.ScheduledStatusSent {
		t.Fatalf("due status = %q, want sent", got.Status)
	}
	if got.SentMessageID != "msg-conv-due" {
		t.Fatalf("sent_message_id = %q, want msg-conv-due", got.SentMessageID)
	}

	still, err := store.GetScheduledMessage(futureItem.ID)
	if err != nil || still == nil {
		t.Fatalf("get future: %v %v", still, err)
	}
	if still.Status != db.ScheduledStatusPending {
		t.Fatalf("future status = %q, want pending", still.Status)
	}
}

// TestTick_MarksFailedOnSenderError checks that a sender error ends up in the
// `failed` bucket with the error message captured.
func TestTick_MarksFailedOnSenderError(t *testing.T) {
	store := newTestStore(t)
	clock := &frozenClock{t: time.Unix(2_000_000, 0)}
	item, _ := store.CreateScheduledMessage(&db.ScheduledMessage{
		ConversationID: "conv-fail",
		Body:           "boom",
		ScheduledAt:    clock.now().Add(-1 * time.Second).UnixMilli(),
	})

	sender := SenderFunc(func(string, string, string) (string, error) {
		return "", errors.New("upstream offline")
	})

	Tick(context.Background(), store, sender, Config{Now: clock.now, Logger: zerolog.Nop()})

	got, _ := store.GetScheduledMessage(item.ID)
	if got.Status != db.ScheduledStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Error != "upstream offline" {
		t.Fatalf("error = %q, want upstream offline", got.Error)
	}
}

// TestTick_SkipsCancelled ensures a row that was cancelled between insert and
// tick doesn't fire.
func TestTick_SkipsCancelled(t *testing.T) {
	store := newTestStore(t)
	clock := &frozenClock{t: time.Unix(3_000_000, 0)}

	it, _ := store.CreateScheduledMessage(&db.ScheduledMessage{
		ConversationID: "conv-cancel",
		Body:           "nope",
		ScheduledAt:    clock.now().Add(-1 * time.Second).UnixMilli(),
	})
	if _, err := store.CancelScheduledMessage(it.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	var sent int
	sender := SenderFunc(func(string, string, string) (string, error) {
		sent++
		return "ok", nil
	})

	Tick(context.Background(), store, sender, Config{Now: clock.now, Logger: zerolog.Nop()})

	if sent != 0 {
		t.Fatalf("sent = %d, want 0 (cancelled row fired)", sent)
	}
	got, _ := store.GetScheduledMessage(it.ID)
	if got.Status != db.ScheduledStatusCancelled {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
}

// TestRun_PollsUntilContextCancelled verifies the loop dispatches on the
// interval tick and stops when ctx is cancelled.
func TestRun_PollsUntilContextCancelled(t *testing.T) {
	store := newTestStore(t)
	clock := &frozenClock{t: time.Now()}

	var mu sync.Mutex
	var calls int
	sender := SenderFunc(func(convID, body, replyToID string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return fmt.Sprintf("msg-%d", calls), nil
	})

	// Due immediately on first-poll.
	if _, err := store.CreateScheduledMessage(&db.ScheduledMessage{
		ConversationID: "conv-run",
		Body:           "go",
		ScheduledAt:    clock.now().Add(-1 * time.Second).UnixMilli(),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, store, sender, Config{
			Interval: 20 * time.Millisecond,
			Now:      clock.now,
			Logger:   zerolog.Nop(),
		})
		close(done)
	}()

	// Wait for the immediate first-poll send.
	deadline := time.After(1 * time.Second)
	for {
		mu.Lock()
		c := calls
		mu.Unlock()
		if c >= 1 {
			break
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("scheduler never fired (calls=%d)", c)
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
