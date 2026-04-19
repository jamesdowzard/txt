package importer

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxghenis/openmessage/internal/db"

	_ "modernc.org/sqlite"
)

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
		`CREATE TABLE message (ROWID INTEGER PRIMARY KEY, guid TEXT, text TEXT, date INTEGER, is_from_me INTEGER, handle_id INTEGER)`,
		`CREATE TABLE chat_message_join (chat_id INTEGER, message_id INTEGER)`,

		`INSERT INTO handle VALUES (1, '+15551234567', '+1 555 1234567', 'iMessage')`,
		`INSERT INTO chat VALUES (1, 'iMessage;-;+15551234567', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (1, 1)`,
		`INSERT INTO message VALUES (1, 'msg-guid-1', 'hello world', 700000000000000000, 0, 1)`,
		`INSERT INTO chat_message_join VALUES (1, 1)`,

		`INSERT INTO handle VALUES (2, '+15557654321', '+1 555 7654321', 'iMessage')`,
		`INSERT INTO chat VALUES (2, 'iMessage;-;+15557654321', '', 45)`,
		`INSERT INTO chat_handle_join VALUES (2, 2)`,
		`INSERT INTO message VALUES (2, 'msg-guid-2', 'second chat', 700000000000000001, 1, 2)`,
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
