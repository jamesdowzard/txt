// Package scheduler polls the scheduled_messages table and dispatches due
// rows via a pluggable Sender. The loop ticks every DefaultPollInterval; on
// each tick it grabs pending rows whose scheduled_at <= now, calls Sender.Send,
// and updates the row to `sent` or `failed` accordingly.
//
// No retry queue, no backoff — a failed scheduled send stays in `failed` for
// the user to decide what to do via the outbox UI. If the process restarts
// mid-send, pending rows get picked up again on next poll (at-least-once,
// but in practice duplicates are rare since the row gets Marked before any
// external acknowledgement).
package scheduler

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/jamesdowzard/txt/internal/db"
)

// DefaultPollInterval is the tick rate. 30s matches the spec and keeps SQLite
// pressure negligible for a personal app.
const DefaultPollInterval = 30 * time.Second

// Sender is what the scheduler calls for each due row. Returning a non-empty
// messageID + nil error = success. Any error = failure (row → failed).
type Sender interface {
	SendText(conversationID, body, replyToID string) (messageID string, err error)
}

// SenderFunc adapts a bare function to the Sender interface. Handy for tests.
type SenderFunc func(conversationID, body, replyToID string) (string, error)

// SendText implements Sender.
func (f SenderFunc) SendText(conversationID, body, replyToID string) (string, error) {
	return f(conversationID, body, replyToID)
}

// Config tunes Run. Zero-value fields fall back to defaults.
type Config struct {
	Interval time.Duration          // default DefaultPollInterval
	Now      func() time.Time       // default time.Now — swap in tests for determinism
	Logger   zerolog.Logger
}

// Run drives the polling loop until ctx is cancelled. It polls once
// immediately on entry so a due row inserted just before app startup fires
// without waiting a full tick.
func Run(ctx context.Context, store *db.Store, sender Sender, cfg Config) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	logger := cfg.Logger

	dispatchOnce(ctx, store, sender, now, logger)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dispatchOnce(ctx, store, sender, now, logger)
		}
	}
}

// dispatchOnce grabs the currently-due pending rows and hands each to the
// sender. Exported as a method-free helper so tests can drive one iteration
// deterministically without spinning a goroutine.
func dispatchOnce(ctx context.Context, store *db.Store, sender Sender, now func() time.Time, logger zerolog.Logger) {
	if ctx.Err() != nil {
		return
	}
	due, err := store.ListDueScheduledMessages(now().UnixMilli())
	if err != nil {
		logger.Error().Err(err).Msg("scheduler: list due failed")
		return
	}
	if len(due) == 0 {
		return
	}
	logger.Info().Int("count", len(due)).Msg("scheduler: dispatching due scheduled messages")
	for _, it := range due {
		if ctx.Err() != nil {
			return
		}
		msgID, err := sender.SendText(it.ConversationID, it.Body, it.ReplyToID)
		if err != nil {
			if markErr := store.MarkScheduledFailed(it.ID, err.Error()); markErr != nil {
				logger.Error().Err(markErr).Int64("id", it.ID).Msg("scheduler: mark failed persist error")
			}
			logger.Warn().Err(err).Int64("id", it.ID).Str("conv_id", it.ConversationID).Msg("scheduler: send failed")
			continue
		}
		if markErr := store.MarkScheduledSent(it.ID, msgID); markErr != nil {
			logger.Error().Err(markErr).Int64("id", it.ID).Msg("scheduler: mark sent persist error")
			continue
		}
		logger.Info().Int64("id", it.ID).Str("conv_id", it.ConversationID).Str("msg_id", msgID).Msg("scheduler: sent")
	}
}

// Tick runs a single dispatch pass. Exposed for tests that want to drive the
// scheduler deterministically without running the loop.
func Tick(ctx context.Context, store *db.Store, sender Sender, cfg Config) {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	dispatchOnce(ctx, store, sender, now, cfg.Logger)
}
