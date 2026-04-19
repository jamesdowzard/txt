package web

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/jamesdowzard/txt/internal/app"
	"github.com/jamesdowzard/txt/internal/client"
	"github.com/jamesdowzard/txt/internal/db"
)

// TextSendResult is what a successful text send returns. MessageID is the
// local-store ID (libgm tmp id / iMessage canonical GUID / WA/Signal id).
type TextSendResult struct {
	MessageID string
}

// TextSender routes a text send to the correct platform backend (Google
// Messages, iMessage, WhatsApp, Signal). It's a thin dispatcher that mirrors
// /api/send's platform switch so the scheduler can reuse it without dragging
// the whole HTTP handler along.
type TextSender struct {
	Store            *db.Store
	Logger           zerolog.Logger
	GetClient        func() *client.Client
	SendWhatsAppText func(conversationID, body, replyToID string) (*db.Message, error)
	SendSignalText   func(conversationID, body, replyToID string) (*db.Message, error)
	RecordOutgoing   func(*db.Message, string) error
	// OnSent is optional; invoked after a successful send so callers can
	// fan SSE events. Called synchronously on the calling goroutine.
	OnSent func(conversationID string)
}

// NewTextSender wires a TextSender from the same APIOptions that APIHandler
// consumes. This is the intended construction path for the scheduler.
func NewTextSender(store *db.Store, logger zerolog.Logger, opts APIOptions) *TextSender {
	getClient := func() *client.Client {
		if opts.Client != nil {
			return opts.Client()
		}
		return nil
	}
	onSent := func(conversationID string) {
		if opts.Events == nil {
			return
		}
		opts.Events.PublishMessages(conversationID)
		opts.Events.PublishConversations()
	}
	return &TextSender{
		Store:            store,
		Logger:           logger,
		GetClient:        getClient,
		SendWhatsAppText: opts.SendWhatsAppText,
		SendSignalText:   opts.SendSignalText,
		RecordOutgoing: func(msg *db.Message, deleteDraftID string) error {
			return store.RecordOutgoingMessage(msg, deleteDraftID)
		},
		OnSent: onSent,
	}
}

// errPlatformUnavailable is returned when a send is requested for a platform
// whose backend isn't wired up (e.g. WhatsApp send in Google-only mode).
var errPlatformUnavailable = errors.New("platform send not available")

// SendText dispatches a text send. Returns the best-effort message ID on
// success. For Google Messages, libgm's SendMessageResponse doesn't carry the
// new ID — MessageID is left empty and the inbound stream fills it in later.
func (s *TextSender) SendText(conversationID, body, replyToID string) (TextSendResult, error) {
	if conversationID == "" || strings.TrimSpace(body) == "" {
		return TextSendResult{}, errors.New("conversation_id and body are required")
	}
	switch platformOf(s.Store, conversationID) {
	case "whatsapp":
		if s.SendWhatsAppText == nil {
			return TextSendResult{}, fmt.Errorf("whatsapp: %w", errPlatformUnavailable)
		}
		msg, err := s.SendWhatsAppText(conversationID, body, replyToID)
		if err != nil {
			return TextSendResult{}, err
		}
		if err := s.RecordOutgoing(msg, ""); err != nil {
			return TextSendResult{}, fmt.Errorf("whatsapp local store: %w", err)
		}
		s.notifySent(conversationID)
		return TextSendResult{MessageID: msg.MessageID}, nil
	case "signal":
		if s.SendSignalText == nil {
			return TextSendResult{}, fmt.Errorf("signal: %w", errPlatformUnavailable)
		}
		msg, err := s.SendSignalText(conversationID, body, replyToID)
		if err != nil {
			return TextSendResult{}, err
		}
		if err := s.RecordOutgoing(msg, ""); err != nil {
			return TextSendResult{}, fmt.Errorf("signal local store: %w", err)
		}
		s.notifySent(conversationID)
		return TextSendResult{MessageID: msg.MessageID}, nil
	case "imessage":
		msg, err := sendIMessageText(s.Store, s.RecordOutgoing, conversationID, body, "")
		if err != nil {
			return TextSendResult{}, err
		}
		s.notifySent(conversationID)
		return TextSendResult{MessageID: msg.MessageID}, nil
	}
	// Default: Google Messages (SMS/RCS).
	cli := s.GetClient()
	if cli == nil {
		return TextSendResult{}, errors.New(app.ErrNotConnected)
	}
	conv, err := cli.GM.GetConversation(conversationID)
	if err != nil {
		return TextSendResult{}, fmt.Errorf("get conversation: %w", err)
	}
	myParticipantID, simPayload := app.ExtractSIMAndParticipant(conv)
	payload := app.BuildSendPayload(conversationID, body, replyToID, myParticipantID, simPayload)
	resp, err := cli.GM.SendMessage(payload)
	if err != nil {
		return TextSendResult{}, fmt.Errorf("send message: %w", err)
	}
	if resp.GetStatus() != gmproto.SendMessageResponse_SUCCESS {
		return TextSendResult{}, fmt.Errorf("send failed: %s", resp.GetStatus().String())
	}
	// Mirror /api/send: stamp a local outgoing row so the UI sees the message
	// immediately. The inbound stream will UPSERT it later with the canonical ID.
	now := time.Now().UnixMilli()
	if err := s.RecordOutgoing(&db.Message{
		MessageID:      payload.TmpID,
		ConversationID: conversationID,
		Body:           body,
		IsFromMe:       true,
		TimestampMS:    now,
		Status:         "OUTGOING_SENDING",
		ReplyToID:      replyToID,
	}, ""); err != nil {
		return TextSendResult{}, fmt.Errorf("gmessages local store: %w", err)
	}
	s.notifySent(conversationID)
	return TextSendResult{MessageID: payload.TmpID}, nil
}

func (s *TextSender) notifySent(conversationID string) {
	if s.OnSent != nil {
		s.OnSent(conversationID)
	}
}

// platformOf is the package-level equivalent of the api.go isFooConversation
// helpers. Kept in one place so scheduler + handler agree on routing.
func platformOf(store *db.Store, conversationID string) string {
	switch {
	case strings.HasPrefix(conversationID, "whatsapp:"):
		return "whatsapp"
	case strings.HasPrefix(conversationID, "signal:"), strings.HasPrefix(conversationID, "signal-group:"):
		return "signal"
	case strings.HasPrefix(conversationID, "imessage:"):
		return "imessage"
	}
	if store == nil {
		return ""
	}
	conv, err := store.GetConversation(conversationID)
	if err != nil || conv == nil {
		return ""
	}
	return conv.SourcePlatform
}
