package importer

import (
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxghenis/openmessage/internal/db"

	_ "modernc.org/sqlite"
)

// makeAttributedBody builds a synthetic NSAttributedString typedstream blob
// matching the layout the parser scans for: ...NSString\x01\x94\x84\x01\x2b<length><utf-8>.
// Length encoding: 1-byte literal if <0x81, else 0x81+u16le, else 0x82+u32le.
func makeAttributedBody(text string) []byte {
	var buf []byte
	buf = append(buf, []byte("streamtyped\x84\x01@\x84\x84\x84NSString\x01\x94\x84\x01\x2b")...)
	switch n := len(text); {
	case n < 0x81:
		buf = append(buf, byte(n))
	case n <= 0xffff:
		buf = append(buf, 0x81)
		var lb [2]byte
		binary.LittleEndian.PutUint16(lb[:], uint16(n))
		buf = append(buf, lb[:]...)
	default:
		buf = append(buf, 0x82)
		var lb [4]byte
		binary.LittleEndian.PutUint32(lb[:], uint32(n))
		buf = append(buf, lb[:]...)
	}
	buf = append(buf, []byte(text)...)
	return buf
}

// TestIMessageImportFromDB_DoesNotDeadlock proves the importer survives the
// "single connection + nested query inside rows.Next()" pattern. The
// pre-fix loadChats called loadChatParticipants while iterating rows, which
// blocked forever because chatDB.SetMaxOpenConns(1) leaves no second conn.
func TestIMessageImportFromDB_DoesNotDeadlock(t *testing.T) {
	tempDir := t.TempDir()
	chatDBPath := filepath.Join(tempDir, "chat.db")

	chatDB, err := sql.Open("sqlite", chatDBPath)
	if err != nil {
		t.Fatalf("open chat.db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT, uncanonicalized_id TEXT, service TEXT)`,
		`CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT, display_name TEXT, style INTEGER)`,
		`CREATE TABLE chat_handle_join (chat_id INTEGER, handle_id INTEGER)`,
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER)`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,

		`INSERT INTO handle VALUES (1, '+15551234567', '+1 555 1234567', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+15551234567', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO message VALUES (1, 'msg-guid-1', 'hello world', NULL, 700000000000000000, 0, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,

		`INSERT INTO handle VALUES (2, '+15557654321', '+1 555 7654321', 'iMessage')`,
		`INSERT INTO chat VALUES (2, 'iMessage;-;+15557654321', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (2, 2)`,
		`INSERT INTO message VALUES (2, 'msg-guid-2', 'second chat', NULL, 700000000000000001, 1, 2)`,
		`INSERT INTO chat_message_join VALUES (2, 2)`,
	}
	for _, s := range stmts {
		if _, err := chatDB.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	chatDB.Close()

	store, err := db.New(filepath.Join(tempDir, "messages.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer store.Close()

	im := &IMessage{DBPath: chatDBPath, MyName: "Me"}

	done := make(chan struct{})
	var result *ImportResult
	var importErr error
	go func() {
		result, importErr = im.ImportFromDB(store)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ImportFromDB deadlocked (timeout 5s)")
	}

	if importErr != nil {
		t.Fatalf("ImportFromDB error: %v", importErr)
	}
	if result.ConversationsCreated != 2 {
		t.Errorf("ConversationsCreated = %d, want 2", result.ConversationsCreated)
	}
	if result.MessagesImported != 2 {
		t.Errorf("MessagesImported = %d, want 2", result.MessagesImported)
	}
}

func TestExtractAttributedBodyText(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"empty", nil, ""},
		{"no marker", []byte("nothing here"), ""},
		{"short", makeAttributedBody("Hi"), "Hi"},
		{"medium", makeAttributedBody("I can call and setup you up with my claude account when you are ready"), "I can call and setup you up with my claude account when you are ready"},
		{"two-byte length", makeAttributedBody(strings.Repeat("x", 0x81)), strings.Repeat("x", 0x81)},
		{"large", makeAttributedBody(strings.Repeat("y", 70_000)), strings.Repeat("y", 70_000)},
		{"truncated length prefix", []byte("NSString\x01\x2b\x81"), ""},
		{"truncated body", []byte("NSString\x01\x2b\x10short"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractAttributedBodyText(c.in)
			if got != c.want {
				t.Errorf("got %q want %q", truncate(got, 60), truncate(c.want, 60))
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func TestNormaliseAttachmentPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"~/Library/Messages/Attachments/0f/15/UUID/IMG.heic", "0f/15/UUID/IMG.heic"},
		{filepath.Join(home, "Library/Messages/Attachments/0f/15/UUID/IMG.heic"), "0f/15/UUID/IMG.heic"},
		{"/tmp/elsewhere.png", ""},
		{"~/Library/Messages/Attachments/../../../etc/passwd", ""},
		{"~/Library/Messages/Attachments/", ""},
	}
	for _, c := range cases {
		got := normaliseAttachmentPath(c.in)
		if got != c.want {
			t.Errorf("normaliseAttachmentPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIMessageImportFromDB_AttributedBodyFallback proves loadMessages reads
// message.attributedBody when message.text is NULL (the modern Messages.app
// layout). Pre-fix: rows with NULL text were silently dropped.
func TestIMessageImportFromDB_AttributedBodyFallback(t *testing.T) {
	tempDir := t.TempDir()
	chatDBPath := filepath.Join(tempDir, "chat.db")

	chatDB, err := sql.Open("sqlite", chatDBPath)
	if err != nil {
		t.Fatalf("open chat.db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT, uncanonicalized_id TEXT, service TEXT)`,
		`CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT, display_name TEXT, style INTEGER)`,
		`CREATE TABLE chat_handle_join (chat_id INTEGER, handle_id INTEGER)`,
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER)`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,
		`INSERT INTO handle VALUES (1, '+15551234567', '+1 555 1234567', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+15551234567', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 2)`,
		`INSERT INTO chat_message_join VALUES (1, 3)`,
	}
	for _, s := range stmts {
		if _, err := chatDB.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	// Three messages: incoming with text, outgoing with attributedBody only,
	// outgoing with neither (sticker placeholder — should be skipped).
	rows := []struct {
		rowid    int
		guid     string
		text     interface{}
		body     interface{}
		isFromMe int
	}{
		{1, "msg-incoming", "Step 1 (done) - Purchase laptop", nil, 0},
		{2, "msg-outgoing", nil, makeAttributedBody("I can call and setup you up with my claude account when you are ready"), 1},
		{3, "msg-sticker", nil, nil, 1},
	}
	for i, r := range rows {
		_, err := chatDB.Exec(
			`INSERT INTO message VALUES (?, ?, ?, ?, ?, ?, 1)`,
			r.rowid, r.guid, r.text, r.body, 700000000000000000+int64(i), r.isFromMe,
		)
		if err != nil {
			t.Fatalf("insert message %d: %v", r.rowid, err)
		}
	}
	chatDB.Close()

	store, err := db.New(filepath.Join(tempDir, "messages.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer store.Close()

	im := &IMessage{DBPath: chatDBPath, MyName: "Me"}
	result, err := im.ImportFromDB(store)
	if err != nil {
		t.Fatalf("ImportFromDB: %v", err)
	}
	if result.MessagesImported != 2 {
		t.Errorf("MessagesImported = %d, want 2 (sticker row dropped)", result.MessagesImported)
	}
	// Confirm the outgoing message landed with body extracted from attributedBody.
	got, err := store.GetMessageByID("imessage:msg-outgoing")
	if err != nil {
		t.Fatalf("GetMessage outgoing: %v", err)
	}
	want := "I can call and setup you up with my claude account when you are ready"
	if got.Body != want {
		t.Errorf("outgoing body = %q, want %q", got.Body, want)
	}
}

// TestIMessageImportFromDB_AttachmentMetadata proves loadMessages joins the
// attachment + message_attachment_join tables and stamps MediaID/MimeType on
// the imported message. Attachment-only rows ("\ufffc" body) keep MediaID and
// drop the placeholder body so the UI just renders the image.
func TestIMessageImportFromDB_AttachmentMetadata(t *testing.T) {
	tempDir := t.TempDir()
	chatDBPath := filepath.Join(tempDir, "chat.db")

	chatDB, err := sql.Open("sqlite", chatDBPath)
	if err != nil {
		t.Fatalf("open chat.db: %v", err)
	}
	stmts := []string{
		`CREATE TABLE handle (ROWID INTEGER PRIMARY KEY, id TEXT, uncanonicalized_id TEXT, service TEXT)`,
		`CREATE TABLE chat (ROWID INTEGER PRIMARY KEY, guid TEXT, display_name TEXT, style INTEGER)`,
		`CREATE TABLE chat_handle_join (chat_id INTEGER, handle_id INTEGER)`,
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER)`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,
		`INSERT INTO handle VALUES (1, '+15551234567', '+1 555 1234567', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+15551234567', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 2)`,
		`INSERT INTO chat_message_join VALUES (1, 3)`,
		// Image-only message from Tommi (object-replacement char in body)
		`INSERT INTO message VALUES (1, 'msg-img-only', char(0xfffc), NULL, 700000000000000000, 0, 1)`,
		`INSERT INTO attachment VALUES (10, 'att-1', '~/Library/Messages/Attachments/0f/15/UUID/IMG_9046.heic', 'image/heic', 0)`,
		`INSERT INTO message_attachment_join VALUES (1, 10)`,
		// Text-with-image message
		`INSERT INTO message VALUES (2, 'msg-text-img', 'Check this out', NULL, 700000000000000001, 1, 1)`,
		`INSERT INTO attachment VALUES (11, 'att-2', '~/Library/Messages/Attachments/aa/bb/UUID2/snap.png', 'image/png', 0)`,
		`INSERT INTO message_attachment_join VALUES (2, 11)`,
		// Hidden attachment (e.g. Memoji metadata) → MediaID should stay empty
		`INSERT INTO message VALUES (3, 'msg-hidden-att', 'Hi', NULL, 700000000000000002, 0, 1)`,
		`INSERT INTO attachment VALUES (12, 'att-3', '~/Library/Messages/Attachments/cc/dd/UUID3/hidden.dat', 'application/octet-stream', 1)`,
		`INSERT INTO message_attachment_join VALUES (3, 12)`,
	}
	for _, s := range stmts {
		if _, err := chatDB.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
	chatDB.Close()

	store, err := db.New(filepath.Join(tempDir, "messages.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer store.Close()

	im := &IMessage{DBPath: chatDBPath, MyName: "Me"}
	result, err := im.ImportFromDB(store)
	if err != nil {
		t.Fatalf("ImportFromDB: %v", err)
	}
	if result.MessagesImported != 3 {
		t.Errorf("MessagesImported = %d, want 3", result.MessagesImported)
	}

	imgOnly, _ := store.GetMessageByID("imessage:msg-img-only")
	if imgOnly == nil {
		t.Fatal("img-only message not imported")
	}
	if imgOnly.Body != "" {
		t.Errorf("img-only body = %q, want empty (placeholder stripped)", imgOnly.Body)
	}
	if want := "0f/15/UUID/IMG_9046.heic"; imgOnly.MediaID != want {
		t.Errorf("img-only MediaID = %q, want %q", imgOnly.MediaID, want)
	}
	if imgOnly.MimeType != "image/heic" {
		t.Errorf("img-only MimeType = %q, want image/heic", imgOnly.MimeType)
	}

	textImg, _ := store.GetMessageByID("imessage:msg-text-img")
	if textImg == nil {
		t.Fatal("text-with-image not imported")
	}
	if textImg.Body != "Check this out" {
		t.Errorf("text-img body = %q, want 'Check this out'", textImg.Body)
	}
	if textImg.MediaID == "" || textImg.MimeType != "image/png" {
		t.Errorf("text-img media metadata missing: id=%q mime=%q", textImg.MediaID, textImg.MimeType)
	}

	hidden, _ := store.GetMessageByID("imessage:msg-hidden-att")
	if hidden == nil {
		t.Fatal("hidden-attachment message not imported")
	}
	if hidden.MediaID != "" {
		t.Errorf("hidden attachment leaked into MediaID: %q", hidden.MediaID)
	}
}
