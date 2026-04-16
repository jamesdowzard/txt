package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/db"
)

const (
	outboxTickInterval = 10 * time.Second
	outboxMaxAttempts  = 5 // give up after 5 attempts → status='failed'
)

// StartOutboxDispatcher begins a background ticker that scans the outbox table
// for pending items whose send_at <= now, and dispatches each via the active
// libgm client. Calls cancel(ctx) to stop. Safe to call once per process.
func (a *App) StartOutboxDispatcher(ctx context.Context) {
	go a.runOutboxLoop(ctx)
}

func (a *App) runOutboxLoop(ctx context.Context) {
	ticker := time.NewTicker(outboxTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDueOutboxItems()
		}
	}
}

// dispatchDueOutboxItems is exported as a method-receiver helper for tests; call
// it directly to flush without waiting for the ticker.
func (a *App) dispatchDueOutboxItems() {
	cli := a.GetClient()
	if cli == nil {
		return // no point trying — items stay pending
	}
	due, err := a.Store.ListDueOutboxItems()
	if err != nil {
		a.Logger.Error().Err(err).Msg("outbox: list due items failed")
		return
	}
	if len(due) == 0 {
		return
	}
	a.Logger.Info().Int("count", len(due)).Msg("outbox: dispatching due items")

	var wg sync.WaitGroup
	for _, it := range due {
		wg.Add(1)
		go func(it *db.OutboxItem) {
			defer wg.Done()
			a.dispatchOutboxItem(it)
		}(it)
	}
	wg.Wait()
}

// dispatchOutboxItem sends one item via libgm. Mirrors /api/send for SMS/RCS
// (Google Messages). WhatsApp/Signal scheduling is out of scope — those routes
// would need their own send paths and aren't covered by this MVP.
func (a *App) dispatchOutboxItem(it *db.OutboxItem) {
	cli := a.GetClient()
	if cli == nil {
		_ = a.Store.IncrementOutboxAttempts(it.ID, "client offline at dispatch")
		return
	}
	conv, err := cli.GM.GetConversation(it.ConversationID)
	if err != nil {
		a.outboxRetryOrFail(it, fmt.Sprintf("get conversation: %v", err))
		return
	}
	myParticipantID, simPayload := ExtractSIMAndParticipant(conv)
	payload := BuildSendPayload(it.ConversationID, it.Body, "", myParticipantID, simPayload)
	resp, err := cli.GM.SendMessage(payload)
	if err != nil {
		a.outboxRetryOrFail(it, fmt.Sprintf("send: %v", err))
		return
	}
	if resp.GetStatus() != gmproto.SendMessageResponse_SUCCESS {
		a.outboxRetryOrFail(it, fmt.Sprintf("server status %s", resp.GetStatus()))
		return
	}
	// libgm SendMessageResponse doesn't include the new message ID — the inbound
	// stream delivers it as a separate event. Leave sent_message_id blank for now.
	if err := a.Store.MarkOutboxSent(it.ID, ""); err != nil {
		a.Logger.Error().Err(err).Int64("id", it.ID).Msg("outbox: mark sent failed")
		return
	}
	a.Logger.Info().Int64("id", it.ID).Str("conv_id", it.ConversationID).Msg("outbox: sent")
	a.emitMessagesChange(it.ConversationID)
	a.emitConversationsChange()
}

func (a *App) outboxRetryOrFail(it *db.OutboxItem, errMsg string) {
	if it.Attempts+1 >= outboxMaxAttempts {
		_ = a.Store.MarkOutboxFailed(it.ID, errMsg)
		a.Logger.Error().Int64("id", it.ID).Str("error", errMsg).Msg("outbox: gave up after max attempts")
		return
	}
	_ = a.Store.IncrementOutboxAttempts(it.ID, errMsg)
	a.Logger.Warn().Int64("id", it.ID).Int("attempts", it.Attempts+1).Str("error", errMsg).Msg("outbox: dispatch failed, will retry")
}
