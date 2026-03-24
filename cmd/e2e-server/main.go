package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/db"
	"github.com/maxghenis/openmessage/internal/web"
)

const (
	defaultPort        = 7010
	pagedConversation  = "conv-paged"
	pagedConversationN = 150
)

func main() {
	logger := zerolog.Nop()
	store, err := db.New(":memory:")
	if err != nil {
		panic(err)
	}
	defer store.Close()

	if err := seedFixture(store); err != nil {
		panic(err)
	}

	events := web.NewEventBroker()
	base := web.APIHandlerWithOptions(store, nil, logger, nil, web.APIOptions{
		Events:      events,
		IsConnected: func() bool { return true },
	})

	var nextID atomic.Int64
	nextID.Store(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/_e2e/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Body           string `json:"body"`
			ConversationID string `json:"conversation_id"`
			IsFromMe       bool   `json:"is_from_me"`
			SenderName     string `json:"sender_name"`
			SenderNumber   string `json:"sender_number"`
			TimestampMS    int64  `json:"timestamp_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ConversationID == "" || req.Body == "" {
			http.Error(w, "conversation_id and body are required", http.StatusBadRequest)
			return
		}
		if req.TimestampMS == 0 {
			req.TimestampMS = time.Now().UnixMilli()
		}
		msg, err := upsertSyntheticMessage(store, req.ConversationID, req.Body, req.TimestampMS, req.IsFromMe, req.SenderName, req.SenderNumber, nextID.Add(1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events.PublishMessages(req.ConversationID)
		events.PublishConversations()
		writeJSON(w, map[string]any{
			"message_id": msg.MessageID,
			"success":    true,
		})
	})

	mux.HandleFunc("/_e2e/drafts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Body           string `json:"body"`
			ConversationID string `json:"conversation_id"`
			DraftID        string `json:"draft_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ConversationID == "" || req.Body == "" {
			http.Error(w, "conversation_id and body are required", http.StatusBadRequest)
			return
		}
		if req.DraftID == "" {
			req.DraftID = fmt.Sprintf("draft-%d", nextID.Add(1))
		}
		if err := store.UpsertDraft(&db.Draft{
			DraftID:        req.DraftID,
			ConversationID: req.ConversationID,
			Body:           req.Body,
			CreatedAt:      time.Now().UnixMilli(),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events.PublishDrafts(req.ConversationID)
		writeJSON(w, map[string]any{
			"draft_id": req.DraftID,
			"success":  true,
		})
	})

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			base.ServeHTTP(w, r)
			return
		}
		var req struct {
			ConversationID string `json:"conversation_id"`
			Message        string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ConversationID == "" || req.Message == "" {
			http.Error(w, "conversation_id and message are required", http.StatusBadRequest)
			return
		}
		msg, err := upsertSyntheticMessage(store, req.ConversationID, req.Message, time.Now().UnixMilli(), true, "Me", "+15551234567", nextID.Add(1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events.PublishMessages(req.ConversationID)
		events.PublishConversations()
		writeJSON(w, map[string]any{
			"message_id": msg.MessageID,
			"status":     "SUCCESS",
			"success":    true,
		})
	})

	mux.HandleFunc("/api/drafts/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			base.ServeHTTP(w, r)
			return
		}
		var req struct {
			Body    string `json:"body"`
			DraftID string `json:"draft_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.DraftID == "" || req.Body == "" {
			http.Error(w, "draft_id and body are required", http.StatusBadRequest)
			return
		}
		draft, err := store.GetDraft(req.DraftID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if draft == nil {
			http.Error(w, "draft not found", http.StatusNotFound)
			return
		}
		msg, err := upsertSyntheticMessage(store, draft.ConversationID, req.Body, time.Now().UnixMilli(), true, "Me", "+15551234567", nextID.Add(1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := store.DeleteDraft(req.DraftID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events.PublishMessages(draft.ConversationID)
		events.PublishDrafts(draft.ConversationID)
		events.PublishConversations()
		writeJSON(w, map[string]any{
			"message_id": msg.MessageID,
			"status":     "SUCCESS",
			"success":    true,
		})
	})

	mux.Handle("/", base)

	addr := "127.0.0.1:" + strconv.Itoa(serverPort())
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func seedFixture(store *db.Store) error {
	if err := store.SeedDemo(); err != nil {
		return err
	}
	if err := store.UpsertConversation(&db.Conversation{
		ConversationID: pagedConversation,
		Name:           "Paged Thread",
		Participants:   `[{"name":"Pat Page","number":"+15550001111"}]`,
		LastMessageTS:  pagedMessageTimestamp(pagedConversationN),
		SourcePlatform: "sms",
	}); err != nil {
		return err
	}
	for i := 1; i <= pagedConversationN; i++ {
		if err := store.UpsertMessage(&db.Message{
			MessageID:      fmt.Sprintf("paged-%03d", i),
			ConversationID: pagedConversation,
			SenderName:     pagedSenderName(i),
			SenderNumber:   pagedSenderNumber(i),
			Body:           fmt.Sprintf("Paged message %03d", i),
			TimestampMS:    pagedMessageTimestamp(i),
			Status:         "delivered",
			IsFromMe:       i%2 == 0,
			SourcePlatform: "sms",
		}); err != nil {
			return err
		}
	}
	return nil
}

func upsertSyntheticMessage(store *db.Store, conversationID, body string, timestampMS int64, isFromMe bool, senderName, senderNumber string, id int64) (*db.Message, error) {
	msg := &db.Message{
		MessageID:      fmt.Sprintf("e2e-%d", id),
		ConversationID: conversationID,
		SenderName:     senderName,
		SenderNumber:   senderNumber,
		Body:           body,
		TimestampMS:    timestampMS,
		Status:         syntheticStatus(isFromMe),
		IsFromMe:       isFromMe,
		SourcePlatform: "sms",
	}
	if err := store.UpsertMessage(msg); err != nil {
		return nil, err
	}

	conv, err := store.GetConversation(conversationID)
	if err != nil {
		conv = &db.Conversation{
			ConversationID: conversationID,
			Name:           senderName,
			Participants:   "[]",
			SourcePlatform: "sms",
		}
	}
	conv.LastMessageTS = timestampMS
	if !isFromMe {
		conv.UnreadCount++
	}
	if err := store.UpsertConversation(conv); err != nil {
		return nil, err
	}
	return msg, nil
}

func pagedMessageTimestamp(i int) int64 {
	base := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC).UnixMilli()
	return base + int64(i*60_000)
}

func pagedSenderName(i int) string {
	if i%2 == 0 {
		return "Me"
	}
	return "Pat Page"
}

func pagedSenderNumber(i int) string {
	if i%2 == 0 {
		return "+15551234567"
	}
	return "+15550001111"
}

func serverPort() int {
	if raw := os.Getenv("OPENMESSAGES_E2E_PORT"); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}
	return defaultPort
}

func syntheticStatus(isFromMe bool) string {
	if isFromMe {
		return "OUTGOING_COMPLETE"
	}
	return "delivered"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
