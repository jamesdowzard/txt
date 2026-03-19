package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/client"
	"github.com/maxghenis/openmessage/internal/db"
)

// Backfill fetches existing conversations and recent messages from
// Google Messages and stores them in the local database.
func (a *App) Backfill() error {
	cli := a.GetClient()
	if cli == nil {
		return fmt.Errorf("client not connected")
	}

	a.Logger.Info().Msg("Starting backfill of conversations and messages")

	resp, err := cli.GM.ListConversations(100, gmproto.ListConversationsRequest_INBOX)
	if err != nil {
		return fmt.Errorf("list conversations: %w", err)
	}

	convos := resp.GetConversations()
	a.Logger.Info().Int("count", len(convos)).Msg("Fetched conversations")

	for _, conv := range convos {
		if err := a.storeConversation(conv); err != nil {
			a.Logger.Error().Err(err).Str("conv_id", conv.GetConversationID()).Msg("Failed to store conversation")
			continue
		}

		msgResp, err := cli.GM.FetchMessages(conv.GetConversationID(), 20, nil)
		if err != nil {
			a.Logger.Warn().Err(err).Str("conv_id", conv.GetConversationID()).Msg("Failed to fetch messages")
			continue
		}

		for _, msg := range msgResp.GetMessages() {
			a.storeMessage(msg)
		}
	}

	a.Logger.Info().Int("conversations", len(convos)).Msg("Backfill complete")
	return nil
}

// DeepBackfill fetches ALL conversations from ALL folders with cursor pagination,
// fetches ALL messages for each conversation, and discovers conversations via
// contacts that may not appear in any folder listing.
func (a *App) DeepBackfill() {
	if !a.deepBackfillRunning.CompareAndSwap(false, true) {
		a.Logger.Warn().Msg("Deep backfill already running")
		return
	}
	a.deepBackfill()
}

func (a *App) deepBackfill() {
	defer a.deepBackfillRunning.Store(false)

	gm := a.getGMClient()
	if gm == nil {
		a.Logger.Error().Msg("Deep backfill: client not connected")
		return
	}

	a.BackfillProgress.reset()
	defer a.BackfillProgress.finish()

	a.Logger.Info().Msg("Starting deep backfill of all messages")

	// Phase A: Paginate ALL folders to discover conversations
	seen := map[string]bool{}
	folders := []gmproto.ListConversationsRequest_Folder{
		gmproto.ListConversationsRequest_INBOX,
		gmproto.ListConversationsRequest_ARCHIVE,
		gmproto.ListConversationsRequest_SPAM_BLOCKED,
	}

	for _, folder := range folders {
		n := a.paginateFolder(gm, folder, seen)
		a.BackfillProgress.add(0, 0, 0, 1)
		a.Logger.Info().
			Str("folder", folder.String()).
			Int("conversations", n).
			Msg("Deep backfill: folder scan complete")
	}

	// Phase B: Deep backfill messages for each discovered conversation
	a.BackfillProgress.setPhase(BackfillPhaseMessages)

	for convID := range seen {
		n := a.deepBackfillConversation(gm, convID)
		a.BackfillProgress.add(0, n, 0, 0)
	}

	// Phase C: Contact-based discovery for orphan phone numbers
	a.BackfillProgress.setPhase(BackfillPhaseContacts)
	a.discoverFromContacts(gm, seen)

	progress := a.BackfillProgress.snapshot()
	a.Logger.Info().
		Int("conversations", progress.ConversationsFound).
		Int("messages", progress.MessagesFound).
		Int("contacts_checked", progress.ContactsChecked).
		Int("errors", progress.Errors).
		Msg("Deep backfill complete")
}

// paginateFolder fetches all conversations in a folder using cursor pagination.
// It stores each conversation and adds its ID to the seen map. Returns the
// number of new conversations found in this folder.
func (a *App) paginateFolder(gm GMClient, folder gmproto.ListConversationsRequest_Folder, seen map[string]bool) int {
	found := 0
	var cursor *gmproto.Cursor

	for {
		resp, err := gm.ListConversationsWithCursor(100, folder, cursor)
		if err != nil {
			a.Logger.Error().Err(err).Str("folder", folder.String()).Msg("Deep backfill: list conversations failed")
			a.BackfillProgress.addError(fmt.Sprintf("list %s: %v", folder.String(), err))
			break
		}

		convos := resp.GetConversations()
		if len(convos) == 0 {
			break
		}

		batchFound := 0
		batchErrors := 0
		for _, conv := range convos {
			convID := conv.GetConversationID()
			if seen[convID] {
				continue
			}
			seen[convID] = true
			found++

			if err := a.storeConversation(conv); err != nil {
				a.Logger.Error().Err(err).Str("conv_id", convID).Msg("Deep backfill: store conversation failed")
				batchErrors++
				continue
			}
			batchFound++
		}
		a.BackfillProgress.add(batchFound, 0, 0, 0)
		if batchErrors > 0 {
			// Record count but don't spam ErrorDetails with per-conversation store failures
			for range batchErrors {
				a.BackfillProgress.addError("")
			}
		}

		cursor = resp.GetCursor()
		if cursor == nil {
			break
		}

		a.Logger.Debug().
			Str("folder", folder.String()).
			Int("batch", len(convos)).
			Int("found_so_far", found).
			Msg("Deep backfill: fetched conversation batch")
	}

	return found
}

// deepBackfillConversation fetches all messages in a conversation using cursor pagination.
func (a *App) deepBackfillConversation(gm GMClient, convID string) int {
	total := 0
	var cursor *gmproto.Cursor

	for {
		resp, err := gm.FetchMessages(convID, 50, cursor)
		if err != nil {
			a.Logger.Warn().Err(err).Str("conv_id", convID).Msg("Deep backfill: fetch messages failed")
			a.BackfillProgress.addError(fmt.Sprintf("fetch messages %s: %v", convID, err))
			break
		}

		msgs := resp.GetMessages()
		if len(msgs) == 0 {
			break
		}

		for _, msg := range msgs {
			a.storeMessage(msg)
			total++
		}

		cursor = resp.GetCursor()
		if cursor == nil {
			break
		}

		a.Logger.Debug().
			Str("conv_id", convID).
			Int("batch", len(msgs)).
			Int("total_so_far", total).
			Msg("Deep backfill: fetched message batch")
	}

	if total > 0 {
		a.Logger.Info().
			Str("conv_id", convID).
			Int("messages", total).
			Msg("Deep backfill: conversation complete")
	}

	return total
}

// discoverFromContacts lists all contacts and tries to find conversations
// for phone numbers not already seen in the folder scan.
func (a *App) discoverFromContacts(gm GMClient, seen map[string]bool) {
	contactsResp, err := gm.ListContacts()
	if err != nil {
		a.Logger.Warn().Err(err).Msg("Deep backfill: list contacts failed")
		a.BackfillProgress.addError(fmt.Sprintf("list contacts: %v", err))
		return
	}

	contacts := contactsResp.GetContacts()
	a.Logger.Info().Int("count", len(contacts)).Msg("Deep backfill: checking contacts for orphan conversations")

	for _, contact := range contacts {
		num := contact.GetNumber()
		if num == nil || num.GetNumber() == "" {
			continue
		}
		phone := num.GetNumber()

		a.BackfillProgress.add(0, 0, 1, 0)

		convResp, err := gm.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
			Numbers: []*gmproto.ContactNumber{
				{
					MysteriousInt: 2,
					Number:        phone,
					Number2:       phone,
				},
			},
		})
		if err != nil {
			a.Logger.Debug().Err(err).Str("phone", phone).Msg("Deep backfill: GetOrCreateConversation failed for contact")
			a.BackfillProgress.addError("")
			continue
		}

		conv := convResp.GetConversation()
		if conv == nil {
			continue
		}

		convID := conv.GetConversationID()
		if seen[convID] {
			continue
		}
		seen[convID] = true

		if err := a.storeConversation(conv); err != nil {
			a.Logger.Error().Err(err).Str("conv_id", convID).Msg("Deep backfill: store contact conversation failed")
			continue
		}

		n := a.deepBackfillConversation(gm, convID)
		a.BackfillProgress.add(1, n, 0, 0)

		a.Logger.Info().
			Str("phone", phone).
			Str("conv_id", convID).
			Int("messages", n).
			Msg("Deep backfill: discovered conversation via contact")
	}
}

// BackfillConversationByPhone looks up or creates a conversation for a specific
// phone number, stores it, and deep-backfills all its messages.
func (a *App) BackfillConversationByPhone(phone string) error {
	gm := a.getGMClient()
	if gm == nil {
		return fmt.Errorf("client not connected")
	}

	convResp, err := gm.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
		Numbers: NewContactNumbers([]string{phone}),
	})
	if err != nil {
		return fmt.Errorf("get or create conversation: %w", err)
	}

	conv := convResp.GetConversation()
	if conv == nil {
		return fmt.Errorf("no conversation returned for %s", phone)
	}

	if err := a.storeConversation(conv); err != nil {
		return fmt.Errorf("store conversation: %w", err)
	}

	n := a.deepBackfillConversation(gm, conv.GetConversationID())
	a.Logger.Info().
		Str("phone", phone).
		Str("conv_id", conv.GetConversationID()).
		Int("messages", n).
		Msg("Phone backfill complete")

	return nil
}

func (a *App) storeConversation(conv *gmproto.Conversation) error {
	participantsJSON := "[]"
	if ps := conv.GetParticipants(); len(ps) > 0 {
		type pInfo struct {
			Name   string `json:"name"`
			Number string `json:"number"`
			IsMe   bool   `json:"is_me,omitempty"`
		}
		var infos []pInfo
		for _, p := range ps {
			info := pInfo{
				Name: p.GetFullName(),
				IsMe: p.GetIsMe(),
			}
			if id := p.GetID(); id != nil {
				info.Number = id.GetNumber()
			}
			if info.Number == "" {
				info.Number = p.GetFormattedNumber()
			}
			infos = append(infos, info)
		}
		if b, err := json.Marshal(infos); err == nil {
			participantsJSON = string(b)
		}
	}

	unread := 0
	if conv.GetUnread() {
		unread = 1
	}

	return a.Store.UpsertConversation(&db.Conversation{
		ConversationID: conv.GetConversationID(),
		Name:           conv.GetName(),
		IsGroup:        conv.GetIsGroupChat(),
		Participants:   participantsJSON,
		LastMessageTS:  conv.GetLastMessageTimestamp() / 1000,
		UnreadCount:    unread,
	})
}

func (a *App) storeMessage(msg *gmproto.Message) {
	body := client.ExtractMessageBody(msg)
	senderName, senderNumber := client.ExtractSenderInfo(msg)

	status := "unknown"
	if ms := msg.GetMessageStatus(); ms != nil {
		status = ms.GetStatus().String()
	}

	dbMsg := &db.Message{
		MessageID:      msg.GetMessageID(),
		ConversationID: msg.GetConversationID(),
		SenderName:     senderName,
		SenderNumber:   senderNumber,
		Body:           body,
		TimestampMS:    msg.GetTimestamp() / 1000,
		Status:         status,
		IsFromMe:       msg.GetSenderParticipant() != nil && msg.GetSenderParticipant().GetIsMe(),
	}

	if media := client.ExtractMediaInfo(msg); media != nil {
		dbMsg.MediaID = media.MediaID
		dbMsg.MimeType = media.MimeType
		dbMsg.DecryptionKey = hex.EncodeToString(media.DecryptionKey)
	}

	if reactions := client.ExtractReactions(msg); reactions != nil {
		if b, err := json.Marshal(reactions); err == nil {
			dbMsg.Reactions = string(b)
		}
	}
	dbMsg.ReplyToID = client.ExtractReplyToID(msg)

	if err := a.Store.UpsertMessage(dbMsg); err != nil {
		a.Logger.Error().Err(err).Str("msg_id", dbMsg.MessageID).Msg("Failed to store backfill message")
	}
}
