package importer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxghenis/openmessage/internal/db"

	_ "modernc.org/sqlite"
)

// Default path to WhatsApp Desktop's Core Data SQLite database on macOS.
var whatsappDefaultDBPath = filepath.Join(
	os.Getenv("HOME"),
	"Library", "Group Containers",
	"group.net.whatsapp.WhatsApp.shared",
	"ChatStorage.sqlite",
)

// WhatsAppNative imports messages by reading the macOS WhatsApp Desktop database directly.
// This is more robust than the text-export importer because:
//   - No separate bridge process to maintain
//   - WhatsApp Desktop handles its own connection
//   - Always has the latest messages (if the desktop app is synced)
//   - Incremental: only imports messages newer than the last sync
type WhatsAppNative struct {
	// DBPath overrides the default ChatStorage.sqlite location.
	DBPath string
	// MyName is the display name for outgoing messages (default "Me").
	MyName string
	// SinceMS limits import to messages after this Unix millisecond timestamp.
	// When zero, imports everything.
	SinceMS int64
}

// ImportFromDB reads the WhatsApp Desktop database and imports all messages.
// When SinceMS is 0, it auto-detects the latest imported WhatsApp timestamp
// from the store and only imports newer messages (incremental sync).
func (w *WhatsAppNative) ImportFromDB(store *db.Store) (*ImportResult, error) {
	dbPath := w.DBPath
	if dbPath == "" {
		dbPath = whatsappDefaultDBPath
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("WhatsApp Desktop database not found at %s — is WhatsApp Desktop installed?", dbPath)
	}

	// Auto-incremental: if no explicit SinceMS, start from last imported message.
	// Negative SinceMS means "force full import" (reset to 0).
	if w.SinceMS < 0 {
		w.SinceMS = 0
	} else if w.SinceMS == 0 {
		if latest, err := store.LatestTimestamp("whatsapp"); err == nil && latest > 0 {
			// Overlap by 5 minutes to catch any messages that might have been missed
			w.SinceMS = latest - 5*60*1000
		}
	}

	waDB, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open WhatsApp db: %w", err)
	}
	defer waDB.Close()
	waDB.SetMaxOpenConns(1)

	result := &ImportResult{}

	chats, err := w.loadChats(waDB)
	if err != nil {
		return nil, fmt.Errorf("load chats: %w", err)
	}

	for _, chat := range chats {
		convID := "whatsapp:" + chat.jid

		participants, _ := json.Marshal(chat.participants)
		if err := store.UpsertConversation(&db.Conversation{
			ConversationID: convID,
			Name:           chat.name,
			IsGroup:        chat.isGroup,
			Participants:   string(participants),
			LastMessageTS:  chat.lastMessageTS,
			SourcePlatform: "whatsapp",
		}); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("conversation %s: %v", chat.jid, err))
			continue
		}
		result.ConversationsCreated++

		msgs, err := w.loadMessages(waDB, chat.pk, chat.name, chat.isGroup)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("messages for %s: %v", chat.jid, err))
			continue
		}

		for _, m := range msgs {
			msg := &db.Message{
				MessageID:      "whatsapp:" + m.stanzaID,
				ConversationID: convID,
				SenderName:     m.senderName,
				SenderNumber:   m.senderNumber,
				Body:           m.text,
				TimestampMS:    m.timestampMS,
				Status:         "delivered",
				IsFromMe:       m.isFromMe,
				SourcePlatform: "whatsapp",
				SourceID:       m.stanzaID,
			}

			if err := store.UpsertMessage(msg); err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint") {
					result.MessagesDuplicate++
				} else {
					result.Errors = append(result.Errors, fmt.Sprintf("message %s: %v", m.stanzaID, err))
				}
				continue
			}
			result.MessagesImported++
		}
	}

	return result, nil
}

type waChat struct {
	pk            int
	jid           string
	name          string
	isGroup       bool
	participants  []map[string]string
	lastMessageTS int64
}

type waNativeMessage struct {
	stanzaID     string
	text         string
	senderName   string
	senderNumber string
	timestampMS  int64
	isFromMe     bool
}

func (w *WhatsAppNative) loadChats(waDB *sql.DB) ([]waChat, error) {
	rows, err := waDB.Query(`
		SELECT cs.Z_PK, cs.ZCONTACTJID, COALESCE(cs.ZPARTNERNAME, ''),
			cs.ZLASTMESSAGEDATE
		FROM ZWACHATSESSION cs
		WHERE cs.ZCONTACTJID IS NOT NULL
			AND cs.ZREMOVED = 0
			AND cs.ZCONTACTJID NOT LIKE '0@%'
			AND cs.ZCONTACTJID NOT LIKE '%@status'
			AND cs.ZCONTACTJID NOT LIKE '%@broadcast'
		ORDER BY cs.ZLASTMESSAGEDATE DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect all rows first (can't nest queries with MaxOpenConns=1)
	var chats []waChat
	for rows.Next() {
		var c waChat
		var lastDate sql.NullFloat64
		if err := rows.Scan(&c.pk, &c.jid, &c.name, &lastDate); err != nil {
			continue
		}

		c.isGroup = strings.Contains(c.jid, "@g.us")
		if lastDate.Valid {
			c.lastMessageTS = coreDataSecsToMS(lastDate.Float64)
		}

		// Skip chats with no name and no messages
		if c.name == "" && c.lastMessageTS == 0 {
			continue
		}
		if c.name == "" {
			c.name = c.jid
		}

		chats = append(chats, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Second pass: load participants (now safe to query)
	for i := range chats {
		c := &chats[i]
		if c.isGroup {
			c.participants = w.loadGroupMembers(waDB, c.pk)
		} else {
			phone := jidToPhone(c.jid)
			c.participants = []map[string]string{
				{"name": c.name, "number": phone},
			}
		}
	}

	return chats, nil
}

func (w *WhatsAppNative) loadGroupMembers(waDB *sql.DB, chatPK int) []map[string]string {
	rows, err := waDB.Query(`
		SELECT COALESCE(gm.ZMEMBERJID, ''), COALESCE(gm.ZCONTACTNAME, '')
		FROM ZWAGROUPMEMBER gm
		WHERE gm.ZCHATSESSION = ?
	`, chatPK)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var participants []map[string]string
	for rows.Next() {
		var jid, name string
		if err := rows.Scan(&jid, &name); err != nil {
			continue
		}
		phone := jidToPhone(jid)
		if name == "" {
			name = phone
		}
		participants = append(participants, map[string]string{
			"name":   name,
			"number": phone,
		})
	}
	return participants
}

func (w *WhatsAppNative) loadMessages(waDB *sql.DB, chatPK int, chatName string, isGroup bool) ([]waNativeMessage, error) {
	// Build query with optional time filter
	query := `
		SELECT m.ZSTANZAID, COALESCE(m.ZTEXT, ''), m.ZMESSAGEDATE,
			m.ZISFROMME, COALESCE(m.ZFROMJID, ''), COALESCE(m.ZPUSHNAME, '')
		FROM ZWAMESSAGE m
		WHERE m.ZCHATSESSION = ?
			AND m.ZSTANZAID IS NOT NULL
			AND m.ZSTANZAID != ''
	`
	args := []any{chatPK}

	if w.SinceMS > 0 {
		// Convert Unix ms to Core Data seconds
		coreDataSecs := float64(w.SinceMS)/1000.0 - float64(coreDataEpoch)
		query += " AND m.ZMESSAGEDATE > ?"
		args = append(args, coreDataSecs)
	}

	query += " ORDER BY m.ZMESSAGEDATE ASC"

	rows, err := waDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	myName := w.MyName
	if myName == "" {
		myName = "Me"
	}

	var msgs []waNativeMessage
	for rows.Next() {
		var m waNativeMessage
		var date float64
		var fromJID, pushName string
		if err := rows.Scan(&m.stanzaID, &m.text, &date, &m.isFromMe, &fromJID, &pushName); err != nil {
			continue
		}

		// Skip messages with no text content (media-only, system, etc.)
		if m.text == "" {
			continue
		}

		m.timestampMS = coreDataSecsToMS(date)

		if m.isFromMe {
			m.senderName = myName
		} else if !isGroup {
			// 1:1 chat: sender is the chat partner
			m.senderName = chatName
			m.senderNumber = jidToPhone(fromJID)
		} else {
			// Group chat: use fromJID as identifier
			// Push names are encrypted in modern WhatsApp, so use JID-based phone
			m.senderNumber = jidToPhone(fromJID)
			m.senderName = m.senderNumber
			if m.senderName == "" {
				m.senderName = fromJID
			}
		}

		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// coreDataSecsToMS converts a Core Data timestamp (seconds since 2001-01-01) to Unix milliseconds.
func coreDataSecsToMS(secs float64) int64 {
	return int64((secs + float64(coreDataEpoch)) * 1000)
}

// jidToPhone extracts a phone number from a WhatsApp JID.
// e.g., "18312972255@s.whatsapp.net" → "+18312972255"
func jidToPhone(jid string) string {
	if jid == "" {
		return ""
	}
	parts := strings.Split(jid, "@")
	if len(parts) == 0 {
		return ""
	}
	num := parts[0]
	// Only convert numeric JIDs (skip LID format like "12345@lid")
	for _, c := range num {
		if c < '0' || c > '9' {
			return ""
		}
	}
	if num != "" {
		return "+" + num
	}
	return ""
}
