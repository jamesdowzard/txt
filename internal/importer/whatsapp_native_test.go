package importer

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxghenis/openmessage/internal/db"
	"github.com/maxghenis/openmessage/internal/whatsappmedia"

	_ "modernc.org/sqlite"
)

func TestRepairLegacyMediaPlaceholders(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "ChatStorage.sqlite")
	waDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open whatsapp db: %v", err)
	}
	defer waDB.Close()
	if _, err := waDB.Exec(`
		CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZSTANZAID VARCHAR, ZMEDIAITEM INTEGER);
		CREATE TABLE ZWAMEDIAITEM (Z_PK INTEGER PRIMARY KEY, ZMEDIALOCALPATH VARCHAR);
		INSERT INTO ZWAMEDIAITEM (Z_PK, ZMEDIALOCALPATH) VALUES (7, 'Media/jenn/photo.jpg');
		INSERT INTO ZWAMESSAGE (Z_PK, ZSTANZAID, ZMEDIAITEM) VALUES (1, 'abc123', 7);
	`); err != nil {
		t.Fatalf("seed whatsapp db: %v", err)
	}

	mediaPath := filepath.Join(root, "Message", "Media", "jenn")
	if err := os.MkdirAll(mediaPath, 0o755); err != nil {
		t.Fatalf("mkdir media path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaPath, "photo.jpg"), []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}

	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("db.New(): %v", err)
	}
	defer store.Close()
	if err := store.UpsertConversation(&db.Conversation{
		ConversationID: "whatsapp:14699991654@s.whatsapp.net",
		Name:           "Jenn",
		SourcePlatform: "whatsapp",
	}); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	if err := store.UpsertMessage(&db.Message{
		MessageID:      "whatsapp:abc123",
		ConversationID: "whatsapp:14699991654@s.whatsapp.net",
		Body:           "[Photo]",
		TimestampMS:    1,
		SourcePlatform: "whatsapp",
		SourceID:       "abc123",
	}); err != nil {
		t.Fatalf("seed placeholder message: %v", err)
	}

	result, err := (&WhatsAppNative{DBPath: dbPath}).RepairLegacyMediaPlaceholders(store)
	if err != nil {
		t.Fatalf("RepairLegacyMediaPlaceholders(): %v", err)
	}
	if result.MessagesRepaired != 1 {
		t.Fatalf("MessagesRepaired = %d, want 1", result.MessagesRepaired)
	}

	msg, err := store.GetMessageByID("whatsapp:abc123")
	if err != nil {
		t.Fatalf("GetMessageByID(): %v", err)
	}
	if msg == nil {
		t.Fatal("expected repaired message")
	}
	if msg.MimeType != "image/jpeg" {
		t.Fatalf("mime_type = %q, want image/jpeg", msg.MimeType)
	}
	relativePath, err := whatsappmedia.DecodeLocalMediaRef(msg.MediaID)
	if err != nil {
		t.Fatalf("DecodeLocalMediaRef(): %v", err)
	}
	if relativePath != "Media/jenn/photo.jpg" {
		t.Fatalf("relative path = %q", relativePath)
	}
}

func TestInferWhatsAppMediaMIME(t *testing.T) {
	if got := inferWhatsAppMediaMIME("Media/jenn/voice.opus", "[Audio]"); got != "audio/ogg" {
		t.Fatalf("got %q, want audio/ogg", got)
	}
	if got := inferWhatsAppMediaMIME("Media/jenn/photo.jpg", "[Photo]"); !strings.HasPrefix(got, "image/") {
		t.Fatalf("got %q, want image/*", got)
	}
}
