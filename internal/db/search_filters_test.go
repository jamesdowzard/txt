package db

import (
	"path/filepath"
	"testing"
	"time"
)

func seedSearchFixtures(t *testing.T) *Store {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.UpsertConversation(&Conversation{ConversationID: "c1", Name: "Alice", LastMessageTS: time.Now().UnixMilli()}); err != nil {
		t.Fatalf("upsert conv: %v", err)
	}

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	fixtures := []Message{
		{MessageID: "m1", ConversationID: "c1", SenderName: "Alice", Body: "hello world", TimestampMS: base, SourcePlatform: "sms"},
		{MessageID: "m2", ConversationID: "c1", SenderName: "Me", Body: "hello back", TimestampMS: base + 60_000, IsFromMe: true, SourcePlatform: "sms"},
		{MessageID: "m3", ConversationID: "c1", SenderName: "Alice", Body: "look at this photo", TimestampMS: base + 120_000, MediaID: "mid-1", MimeType: "image/jpeg", SourcePlatform: "sms"},
		{MessageID: "m4", ConversationID: "c1", SenderName: "Alice", Body: "from gchat", TimestampMS: base + 180_000, SourcePlatform: "gchat"},
	}
	for i := range fixtures {
		if err := store.UpsertMessage(&fixtures[i]); err != nil {
			t.Fatalf("upsert msg: %v", err)
		}
	}
	return store
}

func TestSearchMessagesFiltered_Platform(t *testing.T) {
	store := seedSearchFixtures(t)
	msgs, err := store.SearchMessagesFiltered("", SearchFilters{SourcePlatform: "gchat"}, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m4" {
		t.Fatalf("want only m4 (gchat), got %+v", msgs)
	}
}

func TestSearchMessagesFiltered_HasMedia(t *testing.T) {
	store := seedSearchFixtures(t)
	yes := true
	msgs, err := store.SearchMessagesFiltered("", SearchFilters{HasMedia: &yes}, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m3" {
		t.Fatalf("want only m3 (media), got %+v", msgs)
	}
}

func TestSearchMessagesFiltered_FromMe(t *testing.T) {
	store := seedSearchFixtures(t)
	yes := true
	msgs, err := store.SearchMessagesFiltered("", SearchFilters{FromMe: &yes}, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m2" {
		t.Fatalf("want only m2 (from me), got %+v", msgs)
	}
}

func TestSearchMessagesFiltered_DateRange(t *testing.T) {
	store := seedSearchFixtures(t)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	msgs, err := store.SearchMessagesFiltered("", SearchFilters{
		AfterMS:  base + 90_000,  // excludes m1, m2
		BeforeMS: base + 150_000, // excludes m4
	}, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m3" {
		t.Fatalf("want only m3 (in window), got %+v", msgs)
	}
}

func TestSearchMessagesFiltered_QueryAndFilter(t *testing.T) {
	store := seedSearchFixtures(t)
	yes := true
	msgs, err := store.SearchMessagesFiltered("hello", SearchFilters{FromMe: &yes}, 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(msgs) != 1 || msgs[0].MessageID != "m2" {
		t.Fatalf("want only m2 (from me + matches 'hello'), got %+v", msgs)
	}
}
