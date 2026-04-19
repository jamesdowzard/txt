package importer

import (
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maxghenis/openmessage/internal/contacts"
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,

		`INSERT INTO handle VALUES (1, '+15551234567', '+1 555 1234567', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+15551234567', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO message VALUES (1, 'msg-guid-1', 'hello world', NULL, 700000000000000000, 0, 1, 0, 0, '')`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,

		`INSERT INTO handle VALUES (2, '+15557654321', '+1 555 7654321', 'iMessage')`,
		`INSERT INTO chat VALUES (2, 'iMessage;-;+15557654321', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (2, 2)`,
		`INSERT INTO message VALUES (2, 'msg-guid-2', 'second chat', NULL, 700000000000000001, 1, 2, 0, 0, '')`,
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
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
			`INSERT INTO message VALUES (?, ?, ?, ?, ?, ?, 1, 0, 0, '')`,
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

// TestIMessageImportFromDB_ContactsResolution proves that handle IDs get
// rewritten to the resolved contact name (sender_name on incoming messages,
// display_name on the conversation) when the IMessage importer is given a
// Contacts index.
func TestIMessageImportFromDB_ContactsResolution(t *testing.T) {
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,
		`INSERT INTO handle VALUES (1, '+61437590462', '+61437590462', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+61437590462', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,
		`INSERT INTO message VALUES (1, 'msg-from-tommi', 'Step 1 (done) - Purchase laptop', NULL, 700000000000000000, 0, 1, 0, 0, '')`,
		// Second chat with an UNKNOWN number — sender_name should fall back
		// to the chat.db handle (no spurious "" stamping).
		`INSERT INTO handle VALUES (2, '+61400000000', '+61400000000', 'iMessage')`,
		`INSERT INTO chat VALUES (2, 'iMessage;-;+61400000000', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (2, 2)`,
		`INSERT INTO chat_message_join VALUES (2, 2)`,
		`INSERT INTO message VALUES (2, 'msg-from-unknown', 'Hello', NULL, 700000000000000001, 0, 2, 0, 0, '')`,
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

	idx := contacts.NewIndex()
	idx.Phones[contacts.NormalizePhone("+61437590462")] = "Tommi Yick"
	im := &IMessage{DBPath: chatDBPath, MyName: "Me", Contacts: idx}
	if _, err := im.ImportFromDB(store); err != nil {
		t.Fatalf("ImportFromDB: %v", err)
	}

	tommiMsg, _ := store.GetMessageByID("imessage:msg-from-tommi")
	if tommiMsg == nil {
		t.Fatal("tommi message missing")
	}
	if tommiMsg.SenderName != "Tommi Yick" {
		t.Errorf("tommi sender_name = %q, want 'Tommi Yick'", tommiMsg.SenderName)
	}

	unknownMsg, _ := store.GetMessageByID("imessage:msg-from-unknown")
	if unknownMsg == nil {
		t.Fatal("unknown message missing")
	}
	if unknownMsg.SenderName != "+61400000000" {
		t.Errorf("unknown sender_name = %q, want raw handle", unknownMsg.SenderName)
	}

	tommiConv, _ := store.GetConversation("imessage:iMessage;-;+61437590462")
	if tommiConv == nil {
		t.Fatal("tommi conversation missing")
	}
	if tommiConv.Name != "Tommi Yick" {
		t.Errorf("tommi conv name = %q, want 'Tommi Yick'", tommiConv.Name)
	}

	unknownConv, _ := store.GetConversation("imessage:iMessage;-;+61400000000")
	if unknownConv == nil {
		t.Fatal("unknown conversation missing")
	}
	if unknownConv.Name != "+61400000000" {
		t.Errorf("unknown conv name = %q, want raw handle", unknownConv.Name)
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
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
		`INSERT INTO message VALUES (1, 'msg-img-only', char(0xfffc), NULL, 700000000000000000, 0, 1, 0, 0, '')`,
		`INSERT INTO attachment VALUES (10, 'att-1', '~/Library/Messages/Attachments/0f/15/UUID/IMG_9046.heic', 'image/heic', 0)`,
		`INSERT INTO message_attachment_join VALUES (1, 10)`,
		// Text-with-image message
		`INSERT INTO message VALUES (2, 'msg-text-img', 'Check this out', NULL, 700000000000000001, 1, 1, 0, 0, '')`,
		`INSERT INTO attachment VALUES (11, 'att-2', '~/Library/Messages/Attachments/aa/bb/UUID2/snap.png', 'image/png', 0)`,
		`INSERT INTO message_attachment_join VALUES (2, 11)`,
		// Hidden attachment (e.g. Memoji metadata) → MediaID should stay empty
		`INSERT INTO message VALUES (3, 'msg-hidden-att', 'Hi', NULL, 700000000000000002, 0, 1, 0, 0, '')`,
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

// TestIMessageImportFromDB_Reactions exercises:
//   - 2000-2005 → 6-emoji reactions stamped on the parent message
//   - 3000+ removal cancels the same actor's prior reaction
//   - "p:0/<guid>" associated_message_guid prefix is stripped
//   - read receipt → status="read" on outgoing
func TestIMessageImportFromDB_Reactions(t *testing.T) {
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`CREATE TABLE attachment (ROWID INTEGER PRIMARY KEY, guid TEXT, filename TEXT, mime_type TEXT, hide_attachment INTEGER DEFAULT 0)`,
		`CREATE TABLE message_attachment_join (message_id INTEGER, attachment_id INTEGER)`,
		`INSERT INTO handle VALUES (1, '+61437590462', '+61437590462', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+61437590462', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		// Outgoing parent message that was read on the other end
		`INSERT INTO chat_message_join VALUES (1, 1)`,
		`INSERT INTO message VALUES (1, 'parent-1', 'Step 1 (done) - Purchase laptop', NULL, 700000000000000000, 1, NULL, 1, 0, '')`,
		// Tommi loves it (assoc_type 2000)
		`INSERT INTO chat_message_join VALUES (1, 2)`,
		`INSERT INTO message VALUES (2, 'react-love', NULL, NULL, 700000000000000001, 0, 1, 0, 2000, 'p:0/parent-1')`,
		// Tommi laughs at it (assoc_type 2003)
		`INSERT INTO chat_message_join VALUES (1, 3)`,
		`INSERT INTO message VALUES (3, 'react-laugh', NULL, NULL, 700000000000000002, 0, 1, 0, 2003, 'p:0/parent-1')`,
		// Tommi removes the laugh (assoc_type 3003)
		`INSERT INTO chat_message_join VALUES (1, 4)`,
		`INSERT INTO message VALUES (4, 'react-laugh-remove', NULL, NULL, 700000000000000003, 0, 1, 0, 3003, 'p:0/parent-1')`,
		// Me likes it too (assoc_type 2001 outgoing)
		`INSERT INTO chat_message_join VALUES (1, 5)`,
		`INSERT INTO message VALUES (5, 'react-like-me', NULL, NULL, 700000000000000004, 1, NULL, 0, 2001, 'p:0/parent-1')`,
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

	idx := contacts.NewIndex()
	idx.Phones[contacts.NormalizePhone("+61437590462")] = "Tommi Yick"
	im := &IMessage{DBPath: chatDBPath, MyName: "Me", Contacts: idx}
	if _, err := im.ImportFromDB(store); err != nil {
		t.Fatalf("ImportFromDB: %v", err)
	}

	parent, err := store.GetMessageByID("imessage:parent-1")
	if err != nil || parent == nil {
		t.Fatalf("parent message missing: %v", err)
	}
	if parent.Status != "read" {
		t.Errorf("parent status = %q, want 'read'", parent.Status)
	}
	if parent.Reactions == "" {
		t.Fatal("parent has no reactions JSON")
	}
	// We don't pin the entire JSON shape (sort order isn't guaranteed
	// across map iterations) — just check that the surviving reactions
	// are present and the removed one isn't.
	if !strings.Contains(parent.Reactions, "❤️") {
		t.Errorf("missing ❤️ reaction: %s", parent.Reactions)
	}
	if !strings.Contains(parent.Reactions, "👍") {
		t.Errorf("missing 👍 reaction: %s", parent.Reactions)
	}
	if strings.Contains(parent.Reactions, "😂") {
		t.Errorf("removed 😂 reaction still present: %s", parent.Reactions)
	}
	if !strings.Contains(parent.Reactions, "Tommi Yick") {
		t.Errorf("Tommi not listed as actor: %s", parent.Reactions)
	}
	if !strings.Contains(parent.Reactions, `"Me"`) {
		t.Errorf("Me not listed as actor: %s", parent.Reactions)
	}

	// Reaction rows themselves should NOT be imported as standalone
	// messages.
	if got, _ := store.GetMessageByID("imessage:react-love"); got != nil {
		t.Error("reaction row leaked into messages table")
	}
}

// TestLookupRecentOutgoingGUID covers the post-send chat.db lookup used by
// /api/send to resolve the canonical GUID of a freshly-sent iMessage.
func TestLookupRecentOutgoingGUID(t *testing.T) {
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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, attributedBody BLOB, date INTEGER, is_from_me INTEGER, handle_id INTEGER, is_read INTEGER DEFAULT 0, associated_message_type INTEGER DEFAULT 0, associated_message_guid TEXT DEFAULT '')`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,
		`INSERT INTO handle VALUES (1, '+61437590462', '+61437590462', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+61437590462', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		// Old outgoing message at 700000000000000000 ns post-Core-Data epoch
		// = 2023-03-08 in Unix terms; well before any sane sinceMS for a
		// recent send, so should be filtered out by the date check.
		`INSERT INTO message VALUES (1, 'old-out', 'hi', NULL, 700000000000000000, 1, NULL, 0, 0, '')`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,
	}
	for _, s := range stmts {
		if _, err := chatDB.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}

	// 1) Old-only row + sinceMS in the future → no match.
	guid, err := LookupRecentOutgoingGUID(chatDBPath, "iMessage;-;+61437590462", time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("lookup (no match): %v", err)
	}
	if guid != "" {
		t.Errorf("expected no match, got %q", guid)
	}

	// 2) Insert a fresh outgoing message dated "now"; lookup must find it.
	nowCoreData := (time.Now().Unix() - coreDataEpoch) * 1_000_000_000
	if _, err := chatDB.Exec(`INSERT INTO message VALUES (2, 'fresh-out', 'just sent', NULL, ?, 1, NULL, 0, 0, '')`, nowCoreData); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}
	if _, err := chatDB.Exec(`INSERT INTO chat_message_join VALUES (1, 2)`); err != nil {
		t.Fatalf("join fresh: %v", err)
	}
	chatDB.Close()

	guid, err = LookupRecentOutgoingGUID(chatDBPath, "iMessage;-;+61437590462", time.Now().UnixMilli()-5000)
	if err != nil {
		t.Fatalf("lookup (fresh): %v", err)
	}
	if guid != "fresh-out" {
		t.Errorf("guid = %q, want 'fresh-out'", guid)
	}

	// 3) Wrong chat GUID → empty.
	guid, err = LookupRecentOutgoingGUID(chatDBPath, "iMessage;-;+61999999999", time.Now().UnixMilli()-5000)
	if err != nil {
		t.Fatalf("lookup (wrong chat): %v", err)
	}
	if guid != "" {
		t.Errorf("expected empty for wrong chat, got %q", guid)
	}
}
