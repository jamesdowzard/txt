package importer

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxghenis/openmessage/internal/contacts"
	"github.com/maxghenis/openmessage/internal/db"

	_ "modernc.org/sqlite"
)

// macOS Core Data epoch: 2001-01-01 00:00:00 UTC in Unix seconds.
const coreDataEpoch = 978307200

// iMessageAttachmentsRoot is where Messages.app stores attachment files.
// chat.db records filenames with the literal "~/" prefix; the importer
// normalises them to paths relative to this root so the runtime home dir
// doesn't bake into messages.db.
const iMessageAttachmentsRoot = "Library/Messages/Attachments"

// IMessage imports messages from the macOS Messages chat.db.
// Requires Full Disk Access to read ~/Library/Messages/chat.db.
type IMessage struct {
	// DBPath is the path to chat.db. Defaults to ~/Library/Messages/chat.db.
	DBPath string
	// MyName is the display name for outgoing messages (default "Me").
	MyName string
	// Contacts is an optional pre-built phone→name index. If nil,
	// ImportFromDB loads it from the macOS AddressBook on every call.
	// Tests inject a fake index to avoid touching the real Contacts DB.
	Contacts contacts.Index
}

// ImportFromDB reads the iMessage database and imports all messages.
func (im *IMessage) ImportFromDB(store *db.Store) (*ImportResult, error) {
	dbPath := im.DBPath
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, "Library", "Messages", "chat.db")
	}

	chatDB, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open iMessage db: %w", err)
	}
	defer chatDB.Close()
	chatDB.SetMaxOpenConns(1)

	// Resolve once per import; an empty/missing AddressBook just means we
	// fall back to the raw handle IDs (current behaviour).
	contactIdx := im.Contacts
	if contactIdx == nil {
		if loaded, err := contacts.LoadIndex(); err == nil {
			contactIdx = loaded
		} else {
			contactIdx = contacts.Index{}
		}
	}

	result := &ImportResult{}

	// Get all chats (conversations)
	chats, err := im.loadChats(chatDB, contactIdx)
	if err != nil {
		return nil, fmt.Errorf("load chats: %w", err)
	}

	for _, chat := range chats {
		convID := "imessage:" + chat.guid

		participants, _ := json.Marshal(chat.participants)
		if err := store.UpsertConversation(&db.Conversation{
			ConversationID: convID,
			Name:           chat.displayName,
			IsGroup:        chat.isGroup,
			Participants:   string(participants),
			LastMessageTS:  chat.lastMessageTS,
			SourcePlatform: "imessage",
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("conversation %s: %v", chat.guid, err))
			continue
		}
		result.ConversationsCreated++

		// Import messages for this chat
		msgs, err := im.loadMessages(chatDB, chat.rowID, contactIdx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("messages for %s: %v", chat.guid, err))
			continue
		}

		for _, m := range msgs {
			msg := &db.Message{
				MessageID:      "imessage:" + m.guid,
				ConversationID: convID,
				SenderName:     m.senderName,
				SenderNumber:   m.senderID,
				Body:           m.text,
				TimestampMS:    m.timestampMS,
				Status:         "delivered",
				IsFromMe:       m.isFromMe,
				MediaID:        m.mediaPath,
				MimeType:       m.mimeType,
				SourcePlatform: "imessage",
				SourceID:       m.guid,
			}

			if err := store.UpsertMessage(msg); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint") {
					result.MessagesDuplicate++
				} else {
					result.Errors = append(result.Errors, fmt.Sprintf("message %s: %v", m.guid, err))
				}
				continue
			}
			result.MessagesImported++
		}
	}

	return result, nil
}

type imessageChat struct {
	rowID         int
	guid          string
	displayName   string
	isGroup       bool
	participants  []map[string]string
	lastMessageTS int64
}

type imessageMessage struct {
	guid        string
	text        string
	senderName  string
	senderID    string
	timestampMS int64
	isFromMe    bool
	mediaPath   string // relative to ~/Library/Messages/Attachments
	mimeType    string
}

func (im *IMessage) loadChats(chatDB *sql.DB, contactIdx contacts.Index) ([]imessageChat, error) {
	rows, err := chatDB.Query(`
		SELECT c.ROWID, c.guid, c.display_name, c.style,
			COALESCE(MAX(m.date), 0) as last_date
		FROM chat c
		LEFT JOIN chat_message_join cmj ON c.ROWID = cmj.chat_id
		LEFT JOIN message m ON cmj.message_id = m.ROWID
		GROUP BY c.ROWID
		ORDER BY last_date DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []imessageChat
	for rows.Next() {
		var c imessageChat
		var style int
		var lastDate int64
		if err := rows.Scan(&c.rowID, &c.guid, &c.displayName, &style, &lastDate); err != nil {
			continue
		}
		c.isGroup = style == 43 // iMessage group chat style
		c.lastMessageTS = coreDataToMS(lastDate)
		chats = append(chats, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Drain + close before issuing nested queries: chatDB has SetMaxOpenConns(1)
	// (chat.db SQLite quirk), so loadChatParticipants would block forever
	// waiting for the connection still held by `rows`.
	rows.Close()

	for i := range chats {
		chats[i].participants = im.loadChatParticipants(chatDB, chats[i].rowID)
		// Annotate participants with resolved names so they're visible to
		// downstream consumers (the conversation Participants JSON column
		// keeps both raw number and resolved name).
		for j, p := range chats[i].participants {
			if resolved := contactIdx.Lookup(p["number"]); resolved != "" {
				chats[i].participants[j]["name"] = resolved
			}
		}
		if chats[i].displayName == "" {
			var names []string
			for _, p := range chats[i].participants {
				if n := p["name"]; n != "" {
					names = append(names, n)
				} else if n := p["number"]; n != "" {
					names = append(names, n)
				}
			}
			chats[i].displayName = strings.Join(names, ", ")
		}
	}
	return chats, nil
}

func (im *IMessage) loadChatParticipants(chatDB *sql.DB, chatRowID int) []map[string]string {
	rows, err := chatDB.Query(`
		SELECT h.id, COALESCE(h.uncanonicalized_id, h.id)
		FROM handle h
		JOIN chat_handle_join chj ON h.ROWID = chj.handle_id
		WHERE chj.chat_id = ?
	`, chatRowID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var participants []map[string]string
	for rows.Next() {
		var id, displayID string
		if err := rows.Scan(&id, &displayID); err != nil {
			continue
		}
		participants = append(participants, map[string]string{
			"name":   displayID,
			"number": id,
		})
	}
	return participants
}

func (im *IMessage) loadMessages(chatDB *sql.DB, chatRowID int, contactIdx contacts.Index) ([]imessageMessage, error) {
	rows, err := chatDB.Query(`
		SELECT m.guid, m.text, m.attributedBody, m.date, m.is_from_me,
			COALESCE(h.id, '') as handle_id,
			COALESCE(h.uncanonicalized_id, h.id, '') as handle_display,
			(SELECT a.filename FROM attachment a
			 JOIN message_attachment_join maj ON a.ROWID = maj.attachment_id
			 WHERE maj.message_id = m.ROWID AND a.hide_attachment = 0
			 ORDER BY a.ROWID LIMIT 1) as attachment_filename,
			(SELECT a.mime_type FROM attachment a
			 JOIN message_attachment_join maj ON a.ROWID = maj.attachment_id
			 WHERE maj.message_id = m.ROWID AND a.hide_attachment = 0
			 ORDER BY a.ROWID LIMIT 1) as attachment_mime
		FROM message m
		JOIN chat_message_join cmj ON m.ROWID = cmj.message_id
		LEFT JOIN handle h ON m.handle_id = h.ROWID
		WHERE cmj.chat_id = ?
		ORDER BY m.date ASC
	`, chatRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	myName := im.MyName
	if myName == "" {
		myName = "Me"
	}

	var msgs []imessageMessage
	for rows.Next() {
		var m imessageMessage
		var date int64
		var handleID, handleDisplay string
		var text, attachmentName, attachmentMime sql.NullString
		var attributedBody []byte
		if err := rows.Scan(&m.guid, &text, &attributedBody, &date, &m.isFromMe, &handleID, &handleDisplay, &attachmentName, &attachmentMime); err != nil {
			continue
		}
		if text.Valid {
			m.text = text.String
		}
		if m.text == "" {
			m.text = extractAttributedBodyText(attributedBody)
		}
		// "\ufffc" is the Unicode object-replacement char Messages stamps in
		// the body of an attachment-only message. Strip it so the UI doesn't
		// render a stray placeholder next to the attachment.
		if m.text == "\ufffc" {
			m.text = ""
		}
		if attachmentName.Valid {
			m.mediaPath = normaliseAttachmentPath(attachmentName.String)
		}
		if attachmentMime.Valid {
			m.mimeType = attachmentMime.String
		}
		if m.text == "" && m.mediaPath == "" {
			continue
		}
		m.timestampMS = coreDataToMS(date)
		if m.isFromMe {
			m.senderName = myName
		} else {
			if resolved := contactIdx.Lookup(handleID); resolved != "" {
				m.senderName = resolved
			} else {
				m.senderName = handleDisplay
			}
			m.senderID = handleID
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// normaliseAttachmentPath converts an attachment.filename value from chat.db
// into a path relative to ~/Library/Messages/Attachments, so messages.db is
// portable across home dirs and the /api/media/ handler can re-anchor it.
// Returns "" for paths outside the attachments root or containing ".."
// segments, so a hostile chat.db can't trick the server into reading
// arbitrary files.
func normaliseAttachmentPath(filename string) string {
	if filename == "" {
		return ""
	}
	p := filename
	if strings.HasPrefix(p, "~/") {
		p = p[2:]
	} else if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		p = p[len(home)+1:]
	}
	prefix := iMessageAttachmentsRoot + "/"
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rel := strings.TrimPrefix(p, prefix)
	if rel == "" || strings.Contains(rel, "..") {
		return ""
	}
	return rel
}

// extractAttributedBodyText pulls the UTF-8 body out of a macOS
// NSAttributedString typedstream blob (message.attributedBody).
//
// Modern Messages.app (macOS 11+) leaves message.text NULL and stores
// the body inside the NSKeyedArchiver typedstream blob. The blob embeds
// the plaintext after the NSString class declaration:
//
//	...NSString\x01\x94\x84\x01\x2b<length><utf-8 bytes>...
//
// where <length> is either a single byte (< 0x81), or 0x81 followed by
// a 2-byte little-endian length, or 0x82 followed by a 4-byte little-endian.
// Returns "" when the blob is missing the marker (sticker/attachment-only
// messages have no text payload).
func extractAttributedBodyText(blob []byte) string {
	if len(blob) == 0 {
		return ""
	}
	nsIdx := bytes.Index(blob, []byte("NSString"))
	if nsIdx < 0 {
		return ""
	}
	relIdx := bytes.Index(blob[nsIdx:], []byte{0x01, 0x2b})
	if relIdx < 0 {
		return ""
	}
	pos := nsIdx + relIdx + 2
	if pos >= len(blob) {
		return ""
	}
	var length int
	switch blob[pos] {
	case 0x81:
		if pos+3 > len(blob) {
			return ""
		}
		length = int(binary.LittleEndian.Uint16(blob[pos+1 : pos+3]))
		pos += 3
	case 0x82:
		if pos+5 > len(blob) {
			return ""
		}
		length = int(binary.LittleEndian.Uint32(blob[pos+1 : pos+5]))
		pos += 5
	default:
		length = int(blob[pos])
		pos++
	}
	if length <= 0 || pos+length > len(blob) {
		return ""
	}
	return string(blob[pos : pos+length])
}

// coreDataToMS converts a macOS Core Data timestamp (nanoseconds since 2001-01-01) to milliseconds since Unix epoch.
func coreDataToMS(date int64) int64 {
	if date == 0 {
		return 0
	}
	secs := date / 1_000_000_000
	return (secs + coreDataEpoch) * 1000
}
