package web

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/client"
	"github.com/maxghenis/openmessage/internal/db"
	"github.com/maxghenis/openmessage/internal/story"
)

//go:embed static/*
var staticFS embed.FS

// APIHandler creates the HTTP handler with JSON API routes and static file serving.
// The client may be nil (disconnected state).
// mcpHandler is an optional http.Handler for the MCP SSE endpoint (mounted at /mcp/).
// StartDeepBackfill can optionally launch a guarded background backfill triggered by POST /api/backfill.
// StatusChecker returns whether the backend is connected.
type StatusChecker func() bool

// UnpairFunc deletes the session and disconnects.
type UnpairFunc func() error

// APIOptions holds optional callbacks for the API handler.
type APIOptions struct {
	Client            func() *client.Client
	IsConnected       StatusChecker
	Unpair            UnpairFunc
	StartDeepBackfill func() bool
	BackfillStatus    func() any         // returns a JSON-serializable backfill progress snapshot
	BackfillPhone     func(string) error // targeted backfill for a single phone number
}

// APIHandler creates a handler with minimal options (used by tests).
func APIHandler(store *db.Store, cli *client.Client, logger zerolog.Logger, mcpHandler http.Handler, onDeepBackfill ...func()) http.Handler {
	var cb func() bool
	if len(onDeepBackfill) > 0 {
		cb = func() bool {
			onDeepBackfill[0]()
			return true
		}
	}
	return APIHandlerWithOptions(store, cli, logger, mcpHandler, APIOptions{
		StartDeepBackfill: cb,
	})
}

func APIHandlerWithOptions(store *db.Store, cli *client.Client, logger zerolog.Logger, mcpHandler http.Handler, opts APIOptions) http.Handler {
	mux := http.NewServeMux()
	getClient := func() *client.Client {
		if opts.Client != nil {
			return opts.Client()
		}
		return cli
	}

	mux.HandleFunc("/api/conversations", func(w http.ResponseWriter, r *http.Request) {
		limit := queryInt(r, "limit", 50)
		convos, err := store.ListConversations(limit)
		if err != nil {
			httpError(w, "list conversations: "+err.Error(), 500)
			return
		}
		if convos == nil {
			convos = []*db.Conversation{}
		}
		writeJSON(w, convos)
	})

	mux.HandleFunc("/api/conversations/", func(w http.ResponseWriter, r *http.Request) {
		// Parse: /api/conversations/{id}/messages
		path := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) != 2 || parts[1] != "messages" {
			httpError(w, "not found", 404)
			return
		}
		convID := parts[0]
		limit := queryInt(r, "limit", 100)
		beforeMS := queryInt64(r, "before", 0)
		afterMS := queryInt64(r, "after", 0)
		var msgs []*db.Message
		var err error
		switch {
		case afterMS > 0:
			msgs, err = store.GetMessagesByConversationAfter(convID, afterMS, limit)
		case beforeMS > 0:
			msgs, err = store.GetMessagesByConversationBefore(convID, beforeMS, limit)
		default:
			msgs, err = store.GetMessagesByConversation(convID, limit)
		}
		if err != nil {
			httpError(w, "get messages: "+err.Error(), 500)
			return
		}
		if msgs == nil {
			msgs = []*db.Message{}
		}
		writeJSON(w, msgs)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			httpError(w, "query parameter 'q' is required", 400)
			return
		}
		limit := queryInt(r, "limit", 50)
		msgs, err := store.SearchMessages(q, "", limit)
		if err != nil {
			httpError(w, "search: "+err.Error(), 500)
			return
		}
		if msgs == nil {
			msgs = []*db.Message{}
		}
		writeJSON(w, msgs)
	})

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			ConversationID string `json:"conversation_id"`
			Message        string `json:"message"`
			ReplyToID      string `json:"reply_to_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.ConversationID == "" || req.Message == "" {
			httpError(w, "conversation_id and message are required", 400)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}
		// Fetch conversation to get SIM and participant info
		conv, err := cli.GM.GetConversation(req.ConversationID)
		if err != nil {
			httpError(w, "get conversation: "+err.Error(), 502)
			return
		}

		myParticipantID, simPayload := app.ExtractSIMAndParticipant(conv)

		payload := app.BuildSendPayload(req.ConversationID, req.Message, req.ReplyToID, myParticipantID, simPayload)

		logger.Info().
			Str("conv_id", req.ConversationID).
			Str("participant_id", myParticipantID).
			Bool("has_sim", simPayload != nil).
			Msg("Sending message")

		resp, err := cli.GM.SendMessage(payload)
		if err != nil {
			httpError(w, "send message: "+err.Error(), 502)
			return
		}
		success := resp.GetStatus() == gmproto.SendMessageResponse_SUCCESS
		if success {
			// Store sent message in DB immediately so UI shows it
			now := time.Now().UnixMilli()
			store.UpsertMessage(&db.Message{
				MessageID:      payload.TmpID,
				ConversationID: req.ConversationID,
				Body:           req.Message,
				IsFromMe:       true,
				TimestampMS:    now,
				Status:         "OUTGOING_SENDING",
				ReplyToID:      req.ReplyToID,
			})
			// Bump conversation to top of list
			store.UpdateConversationTimestamp(req.ConversationID, now)
		}
		writeJSON(w, map[string]any{
			"status":  resp.GetStatus().String(),
			"success": success,
		})
	})

	mux.HandleFunc("/api/send-media", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}

		// Parse multipart form (max 10MB)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			httpError(w, "invalid multipart form: "+err.Error(), 400)
			return
		}

		convID := r.FormValue("conversation_id")
		if convID == "" {
			httpError(w, "conversation_id is required", 400)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			httpError(w, "file is required: "+err.Error(), 400)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			httpError(w, "read file: "+err.Error(), 500)
			return
		}

		mime := header.Header.Get("Content-Type")
		if mime == "" {
			mime = "application/octet-stream"
		}

		// Upload media via libgm
		media, err := cli.GM.UploadMedia(data, header.Filename, mime)
		if err != nil {
			httpError(w, "upload media: "+err.Error(), 502)
			return
		}

		// Get SIM and participant info
		conv, err := cli.GM.GetConversation(convID)
		if err != nil {
			httpError(w, "get conversation: "+err.Error(), 502)
			return
		}

		myParticipantID, simPayload := app.ExtractSIMAndParticipant(conv)

		payload := app.BuildSendMediaPayload(convID, media, myParticipantID, simPayload)

		logger.Info().
			Str("conv_id", convID).
			Str("mime", mime).
			Str("filename", header.Filename).
			Int("size", len(data)).
			Msg("Sending media message")

		resp, err := cli.GM.SendMessage(payload)
		if err != nil {
			httpError(w, "send message: "+err.Error(), 502)
			return
		}
		success := resp.GetStatus() == gmproto.SendMessageResponse_SUCCESS
		if success {
			now := time.Now().UnixMilli()
			store.UpsertMessage(&db.Message{
				MessageID:      payload.TmpID,
				ConversationID: convID,
				Body:           "",
				IsFromMe:       true,
				TimestampMS:    now,
				Status:         "OUTGOING_SENDING",
				MediaID:        media.MediaID,
				MimeType:       media.MimeType,
				DecryptionKey:  hex.EncodeToString(media.DecryptionKey),
			})
			store.UpdateConversationTimestamp(convID, now)
		}
		writeJSON(w, map[string]any{
			"status":  resp.GetStatus().String(),
			"success": success,
		})
	})

	mux.HandleFunc("/api/media/", func(w http.ResponseWriter, r *http.Request) {
		msgID := strings.TrimPrefix(r.URL.Path, "/api/media/")
		if msgID == "" {
			httpError(w, "message_id required", 400)
			return
		}
		msg, err := store.GetMessageByID(msgID)
		if err != nil {
			httpError(w, "get message: "+err.Error(), 500)
			return
		}
		if msg == nil || msg.MediaID == "" {
			httpError(w, "no media for this message", 404)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}
		// Decode hex decryption key
		key, err := hex.DecodeString(msg.DecryptionKey)
		if err != nil {
			httpError(w, "invalid decryption key", 500)
			return
		}
		data, err := cli.GM.DownloadMedia(msg.MediaID, key)
		if err != nil {
			httpError(w, "download media: "+err.Error(), 502)
			return
		}
		w.Header().Set("Content-Type", msg.MimeType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})

	mux.HandleFunc("/api/react", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			ConversationID string `json:"conversation_id"`
			MessageID      string `json:"message_id"`
			Emoji          string `json:"emoji"`
			Action         string `json:"action"` // "add", "remove", "switch"; default "add"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.MessageID == "" || req.Emoji == "" {
			httpError(w, "message_id and emoji are required", 400)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}

		// Get SIM payload from conversation
		var sim *gmproto.SIMPayload
		if req.ConversationID != "" {
			if conv, err := cli.GM.GetConversation(req.ConversationID); err == nil {
				_, sim = app.ExtractSIMAndParticipant(conv)
			}
		}

		payload := BuildReactionPayload(req.MessageID, req.Emoji, req.Action, sim)
		resp, err := cli.GM.SendReaction(payload)
		if err != nil {
			httpError(w, "send reaction: "+err.Error(), 502)
			return
		}
		writeJSON(w, map[string]any{
			"success": resp.GetSuccess(),
		})
	})

	mux.HandleFunc("/api/new-conversation", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			PhoneNumber string `json:"phone_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.PhoneNumber == "" {
			httpError(w, "phone_number is required", 400)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}

		convResp, err := cli.GM.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
			Numbers: app.NewContactNumbers([]string{req.PhoneNumber}),
		})
		if err != nil {
			httpError(w, "failed to get/create conversation: "+err.Error(), 502)
			return
		}
		conv := convResp.GetConversation()
		if conv == nil {
			httpError(w, "no conversation returned", 502)
			return
		}

		convoID := conv.GetConversationID()
		name := req.PhoneNumber
		// Try to get a name from participants
		for _, p := range conv.GetParticipants() {
			if !p.GetIsMe() {
				if fn := p.GetFormattedNumber(); fn != "" {
					name = fn
				}
				if cn := p.GetFullName(); cn != "" {
					name = cn
				}
			}
		}

		// Upsert into local DB so it shows in the sidebar
		store.UpsertConversation(&db.Conversation{
			ConversationID: convoID,
			Name:           name,
			LastMessageTS:  time.Now().UnixMilli(),
		})

		writeJSON(w, map[string]any{
			"conversation_id": convoID,
			"name":            name,
		})
	})

	mux.HandleFunc("/api/mark-read", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			ConversationID string `json:"conversation_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.ConversationID == "" {
			httpError(w, "conversation_id is required", 400)
			return
		}
		if err := store.MarkConversationRead(req.ConversationID); err != nil {
			httpError(w, "mark read: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/drafts", func(w http.ResponseWriter, r *http.Request) {
		conversationID := r.URL.Query().Get("conversation_id")
		if conversationID == "" {
			httpError(w, "conversation_id is required", 400)
			return
		}
		drafts, err := store.ListDrafts(conversationID)
		if err != nil {
			httpError(w, "list drafts: "+err.Error(), 500)
			return
		}
		if drafts == nil {
			drafts = []*db.Draft{}
		}
		writeJSON(w, drafts)
	})

	mux.HandleFunc("/api/drafts/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			DraftID string `json:"draft_id"`
			Body    string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.DraftID == "" || req.Body == "" {
			httpError(w, "draft_id and body are required", 400)
			return
		}
		cli := getClient()
		if cli == nil {
			httpError(w, app.ErrNotConnected, 503)
			return
		}

		// Look up the draft to get conversation_id
		draft, err := store.GetDraft(req.DraftID)
		if err != nil {
			httpError(w, "get draft: "+err.Error(), 500)
			return
		}
		if draft == nil {
			httpError(w, "draft not found", 404)
			return
		}

		// Use the same send logic as /api/send
		conv, err := cli.GM.GetConversation(draft.ConversationID)
		if err != nil {
			httpError(w, "get conversation: "+err.Error(), 502)
			return
		}

		myParticipantID, simPayload := app.ExtractSIMAndParticipant(conv)

		payload := app.BuildSendPayload(draft.ConversationID, req.Body, "", myParticipantID, simPayload)

		logger.Info().
			Str("conv_id", draft.ConversationID).
			Str("draft_id", req.DraftID).
			Msg("Sending draft message")

		resp, err := cli.GM.SendMessage(payload)
		if err != nil {
			httpError(w, "send message: "+err.Error(), 502)
			return
		}
		success := resp.GetStatus() == gmproto.SendMessageResponse_SUCCESS
		if success {
			now := time.Now().UnixMilli()
			store.UpsertMessage(&db.Message{
				MessageID:      payload.TmpID,
				ConversationID: draft.ConversationID,
				Body:           req.Body,
				IsFromMe:       true,
				TimestampMS:    now,
				Status:         "OUTGOING_SENDING",
			})
			store.UpdateConversationTimestamp(draft.ConversationID, now)
			store.DeleteDraft(req.DraftID)
		}
		writeJSON(w, map[string]any{
			"status":  resp.GetStatus().String(),
			"success": success,
		})
	})

	mux.HandleFunc("/api/drafts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			httpError(w, "method not allowed", 405)
			return
		}
		draftID := strings.TrimPrefix(r.URL.Path, "/api/drafts/")
		if draftID == "" {
			httpError(w, "draft_id required", 400)
			return
		}
		if err := store.DeleteDraft(draftID); err != nil {
			httpError(w, "delete draft: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/stats/", func(w http.ResponseWriter, r *http.Request) {
		convID := strings.TrimPrefix(r.URL.Path, "/api/stats/")
		if convID == "" {
			httpError(w, "conversation_id required", 400)
			return
		}
		msgs, err := store.GetMessagesByConversation(convID, 100000)
		if err != nil {
			httpError(w, "get messages: "+err.Error(), 500)
			return
		}
		if len(msgs) == 0 {
			httpError(w, "no messages found", 404)
			return
		}
		stats := story.ComputeStats(msgs, nil)
		writeJSON(w, stats)
	})

	mux.HandleFunc("/api/story/", func(w http.ResponseWriter, r *http.Request) {
		convID := strings.TrimPrefix(r.URL.Path, "/api/story/")
		if convID == "" {
			httpError(w, "conversation_id required", 400)
			return
		}
		msgs, err := store.GetMessagesByConversation(convID, 100000)
		if err != nil {
			httpError(w, "get messages: "+err.Error(), 500)
			return
		}
		if len(msgs) == 0 {
			httpError(w, "no messages found", 404)
			return
		}
		apiKey := r.URL.Query().Get("api_key")
		style := r.URL.Query().Get("style")
		s, err := story.Generate(msgs, story.GenerateConfig{
			Style:             style,
			APIKey:            apiKey,
			MaxSampleMessages: 200,
		})
		if err != nil {
			httpError(w, "generate story: "+err.Error(), 500)
			return
		}
		writeJSON(w, s)
	})

	mux.HandleFunc("/api/backfill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		if opts.StartDeepBackfill != nil {
			if !opts.StartDeepBackfill() {
				httpError(w, "deep backfill already running", 409)
				return
			}
			writeJSON(w, map[string]string{"status": "started"})
		} else {
			httpError(w, "deep backfill not available", 501)
		}
	})

	mux.HandleFunc("/api/backfill/status", func(w http.ResponseWriter, r *http.Request) {
		if opts.BackfillStatus != nil {
			writeJSON(w, opts.BackfillStatus())
		} else {
			writeJSON(w, map[string]any{
				"running": false,
				"phase":   "idle",
			})
		}
	})

	mux.HandleFunc("/api/backfill/phone", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		var req struct {
			PhoneNumber string `json:"phone_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "invalid JSON: "+err.Error(), 400)
			return
		}
		if req.PhoneNumber == "" {
			httpError(w, "phone_number is required", 400)
			return
		}
		if opts.BackfillPhone == nil {
			httpError(w, "phone backfill not available", 501)
			return
		}
		if err := opts.BackfillPhone(req.PhoneNumber); err != nil {
			httpError(w, "backfill phone: "+err.Error(), 502)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		connected := getClient() != nil
		if opts.IsConnected != nil {
			connected = opts.IsConnected()
		}
		writeJSON(w, map[string]any{
			"connected": connected,
		})
	})

	mux.HandleFunc("/api/unpair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpError(w, "method not allowed", 405)
			return
		}
		if opts.Unpair != nil {
			if err := opts.Unpair(); err != nil {
				httpError(w, "unpair: "+err.Error(), 500)
				return
			}
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// Serve embedded static files at root
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create static sub-filesystem")
	}
	staticHandler := http.FileServer(http.FS(staticContent))
	mux.Handle("/", staticHandler)

	// Wrap the mux to intercept /mcp/ requests before the mux's catch-all
	if mcpHandler != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/mcp/") {
				mcpHandler.ServeHTTP(w, r)
				return
			}
			mux.ServeHTTP(w, r)
		})
	}

	return mux
}

// BuildReactionPayload constructs a SendReactionRequest using gmproto.MakeReactionData
// for proper emoji type mapping, matching the mautrix bridge format.
func BuildReactionPayload(messageID, emoji, action string, sim *gmproto.SIMPayload) *gmproto.SendReactionRequest {
	var a gmproto.SendReactionRequest_Action
	switch strings.ToLower(action) {
	case "remove":
		a = gmproto.SendReactionRequest_REMOVE
	case "switch":
		a = gmproto.SendReactionRequest_SWITCH
	default:
		a = gmproto.SendReactionRequest_ADD
	}
	return &gmproto.SendReactionRequest{
		MessageID:    messageID,
		ReactionData: gmproto.MakeReactionData(emoji),
		Action:       a,
		SIMPayload:   sim,
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func queryInt64(r *http.Request, key string, defaultVal int64) int64 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}
