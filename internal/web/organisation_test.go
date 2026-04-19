package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jamesdowzard/txt/internal/db"
)

// Helpers: seed a conversation with at least one message so list queries
// (which require a message to exist) return it.
func seedConvWithMessage(t *testing.T, ts *testServer, id, name string, ts_ms int64) {
	t.Helper()
	if err := ts.store.UpsertConversation(&db.Conversation{
		ConversationID: id,
		Name:           name,
		LastMessageTS:  ts_ms,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ts.store.UpsertMessage(&db.Message{
		MessageID:      "m-" + id,
		ConversationID: id,
		Body:           "seed",
		TimestampMS:    ts_ms,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPinConversation(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	resp, err := http.Post(ts.server.URL+"/api/conversations/c1/pin", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("pin = %d, want 200: %s", resp.StatusCode, body)
	}
	var convo db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&convo); err != nil {
		t.Fatal(err)
	}
	if convo.PinnedAt == 0 {
		t.Fatalf("PinnedAt = 0 after pin, want > 0")
	}

	// Unpin
	resp2, err := http.Post(ts.server.URL+"/api/conversations/c1/unpin", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("unpin status = %d", resp2.StatusCode)
	}
	var convo2 db.Conversation
	if err := json.NewDecoder(resp2.Body).Decode(&convo2); err != nil {
		t.Fatal(err)
	}
	if convo2.PinnedAt != 0 {
		t.Fatalf("PinnedAt = %d after unpin, want 0", convo2.PinnedAt)
	}
}

func TestMuteConversationForDuration(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	before := time.Now().Unix()
	resp, err := http.Post(ts.server.URL+"/api/conversations/c1/mute", "application/json", strings.NewReader(`{"duration_ms": 3600000}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("mute = %d, want 200: %s", resp.StatusCode, body)
	}
	var convo db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&convo); err != nil {
		t.Fatal(err)
	}
	if convo.NotificationMode != db.NotificationModeMuted {
		t.Fatalf("NotificationMode = %q, want muted", convo.NotificationMode)
	}
	// muted_until should be roughly before + 3600s (allow a generous slack).
	if convo.MutedUntil < before+3590 || convo.MutedUntil > before+3610 {
		t.Fatalf("MutedUntil = %d, want ~%d (+3600s)", convo.MutedUntil, before+3600)
	}
}

func TestMuteConversationForever(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	// Empty body → forever
	resp, err := http.Post(ts.server.URL+"/api/conversations/c1/mute", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("mute forever = %d, want 200: %s", resp.StatusCode, body)
	}
	var convo db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&convo); err != nil {
		t.Fatal(err)
	}
	if convo.NotificationMode != db.NotificationModeMuted {
		t.Fatalf("NotificationMode = %q, want muted", convo.NotificationMode)
	}
	if convo.MutedUntil != 0 {
		t.Fatalf("MutedUntil = %d, want 0 (forever)", convo.MutedUntil)
	}
	if !convo.IsMuted() {
		t.Fatalf("IsMuted() = false for forever-mute, want true")
	}

	// duration_ms = 0 JSON → forever
	resp2, err := http.Post(ts.server.URL+"/api/conversations/c1/mute", "application/json", strings.NewReader(`{"duration_ms": 0}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("mute duration_ms=0 status = %d", resp2.StatusCode)
	}
	var convo2 db.Conversation
	_ = json.NewDecoder(resp2.Body).Decode(&convo2)
	if convo2.MutedUntil != 0 {
		t.Fatalf("MutedUntil = %d, want 0 for duration_ms=0", convo2.MutedUntil)
	}
}

func TestUnmuteConversation(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	// Mute first
	if err := ts.store.SetConversationMute("c1", time.Now().Unix()+3600); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(ts.server.URL+"/api/conversations/c1/unmute", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unmute = %d, want 200: %s", resp.StatusCode, body)
	}
	var convo db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&convo); err != nil {
		t.Fatal(err)
	}
	if convo.NotificationMode != db.NotificationModeAll {
		t.Fatalf("NotificationMode = %q, want all", convo.NotificationMode)
	}
	if convo.MutedUntil != 0 {
		t.Fatalf("MutedUntil = %d, want 0 after unmute", convo.MutedUntil)
	}
	if convo.IsMuted() {
		t.Fatalf("IsMuted() = true after unmute, want false")
	}
}

func TestMuteConversationRejectsNegativeDuration(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	resp, err := http.Post(ts.server.URL+"/api/conversations/c1/mute", "application/json", strings.NewReader(`{"duration_ms": -1000}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("negative duration = %d, want 400: %s", resp.StatusCode, body)
	}
}

func TestMuteMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	req, _ := http.NewRequest(http.MethodGet, ts.server.URL+"/api/conversations/c1/mute", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mute = %d, want 405", resp.StatusCode)
	}
}

func TestListConversationsExcludesArchivedByDefault(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 200)
	seedConvWithMessage(t, ts, "c2", "Bob", 100)

	// Archive c2 locally (no libgm needed for this path).
	if err := ts.store.SetConversationArchived("c2", true); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.server.URL + "/api/conversations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got []db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ConversationID != "c1" {
		t.Fatalf("default list = %+v, want [c1] only (c2 archived)", got)
	}
}

func TestListConversationsIncludesArchivedWithFlag(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 200)
	seedConvWithMessage(t, ts, "c2", "Bob", 100)

	if err := ts.store.SetConversationArchived("c2", true); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.server.URL + "/api/conversations?include_archived=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("include_archived list len = %d, want 2", len(got))
	}
}

func TestListConversationsPinnedToTop(t *testing.T) {
	ts := newTestServer(t)
	// Older message but pinned should still sort above newer unpinned.
	seedConvWithMessage(t, ts, "old-pinned", "Old Pinned", 100)
	seedConvWithMessage(t, ts, "new-unpinned", "New Unpinned", 9000)
	seedConvWithMessage(t, ts, "older-pinned", "Older Pinned", 50)

	if err := ts.store.SetConversationPinned("old-pinned", true); err != nil {
		t.Fatal(err)
	}
	// Sleep so the second pin gets a later pinned_at (order within pinned group).
	time.Sleep(1100 * time.Millisecond)
	if err := ts.store.SetConversationPinned("older-pinned", true); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.server.URL + "/api/conversations")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []db.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("list len = %d, want 3", len(got))
	}
	// Pinned first (newest pin first), then unpinned by last_message_ts DESC.
	wantOrder := []string{"older-pinned", "old-pinned", "new-unpinned"}
	for i, id := range wantOrder {
		if got[i].ConversationID != id {
			t.Fatalf("position %d = %q, want %q (full: %v)", i, got[i].ConversationID, id, conversationIDs(got))
		}
	}
}

func TestArchivedAtClearedOnUnarchiveLocal(t *testing.T) {
	ts := newTestServer(t)
	seedConvWithMessage(t, ts, "c1", "Alice", 100)

	if err := ts.store.SetConversationArchived("c1", true); err != nil {
		t.Fatal(err)
	}
	stored, err := ts.store.GetConversation("c1")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ArchivedAt == 0 {
		t.Fatalf("ArchivedAt = 0 after archive, want > 0")
	}
	if stored.Folder != db.FolderArchive {
		t.Fatalf("Folder = %q after archive, want %q", stored.Folder, db.FolderArchive)
	}

	if err := ts.store.SetConversationArchived("c1", false); err != nil {
		t.Fatal(err)
	}
	stored2, _ := ts.store.GetConversation("c1")
	if stored2.ArchivedAt != 0 {
		t.Fatalf("ArchivedAt = %d after unarchive, want 0", stored2.ArchivedAt)
	}
	if stored2.Folder != db.FolderInbox {
		t.Fatalf("Folder = %q after unarchive, want %q", stored2.Folder, db.FolderInbox)
	}
}

func conversationIDs(convs []db.Conversation) []string {
	out := make([]string, 0, len(convs))
	for _, c := range convs {
		out = append(out, c.ConversationID)
	}
	return out
}
