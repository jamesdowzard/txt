package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/maxghenis/openmessage/internal/db"
)

func seedConvo(t *testing.T, s *db.Store, convID string, msgs []db.Message) {
	t.Helper()
	if err := s.UpsertConversation(&db.Conversation{
		ConversationID: convID,
		Name:           "Test Conv",
		LastMessageTS:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}
	for i := range msgs {
		m := msgs[i]
		m.ConversationID = convID
		if m.MessageID == "" {
			m.MessageID = convID + "-" + time.Unix(0, m.TimestampMS*int64(time.Millisecond)).UTC().Format("150405.000")
		}
		if err := s.UpsertMessage(&m); err != nil {
			t.Fatalf("upsert message: %v", err)
		}
	}
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestSummarizeConversation(t *testing.T) {
	a := testApp(t)
	now := time.Now().UnixMilli()
	seedConvo(t, a.Store, "c1", []db.Message{
		{SenderName: "Alice", Body: "Hi there!", TimestampMS: now - 3000},
		{SenderName: "Me", Body: "Hey Alice", TimestampMS: now - 2000, IsFromMe: true},
		{SenderName: "Alice", Body: "how are you?", TimestampMS: now - 1000},
	})

	res := callTool(t, summarizeConversationHandler(a), map[string]any{
		"conversation_id": "c1",
	})
	text := resultText(t, res)
	if !strings.Contains(text, "3 messages") {
		t.Errorf("expected '3 messages' in summary, got %q", text)
	}
	if !strings.Contains(text, "Alice") {
		t.Errorf("expected sender 'Alice' in summary, got %q", text)
	}
}

func TestSummarizeConversationMissing(t *testing.T) {
	a := testApp(t)
	res := callTool(t, summarizeConversationHandler(a), map[string]any{"conversation_id": "nope"})
	if !res.IsError {
		t.Fatal("expected error result for missing conversation")
	}
}

func TestFindUnansweredReturnsInboundOnly(t *testing.T) {
	a := testApp(t)
	now := time.Now().UnixMilli()
	seedConvo(t, a.Store, "c-in", []db.Message{
		{SenderName: "Alice", Body: "ping?", TimestampMS: now - 1000},
	})
	seedConvo(t, a.Store, "c-out", []db.Message{
		{SenderName: "Me", Body: "handled", TimestampMS: now - 1000, IsFromMe: true},
	})

	res := callTool(t, findUnansweredHandler(a), map[string]any{})
	text := resultText(t, res)
	if !strings.Contains(text, "c-in") {
		t.Errorf("expected inbound convo 'c-in' in output, got %q", text)
	}
	if strings.Contains(text, "c-out") {
		t.Errorf("outbound-resolved convo 'c-out' should be excluded, got %q", text)
	}
}

func TestSuggestReplyKeyword(t *testing.T) {
	a := testApp(t)
	now := time.Now().UnixMilli()
	seedConvo(t, a.Store, "c1", []db.Message{
		{SenderName: "Alice", Body: "Thanks for the help!", TimestampMS: now - 1000},
	})
	res := callTool(t, suggestReplyHandler(a), map[string]any{"conversation_id": "c1"})
	text := resultText(t, res)
	if !strings.Contains(strings.ToLower(text), "welcome") && !strings.Contains(text, "Anytime") {
		t.Errorf("expected thanks-branch suggestion, got %q", text)
	}
}

func TestExtractActionItemsFindsPatterns(t *testing.T) {
	a := testApp(t)
	now := time.Now().UnixMilli()
	seedConvo(t, a.Store, "c1", []db.Message{
		{SenderName: "Alice", Body: "Can you send me the report? Also please remember to call Bob.", TimestampMS: now - 1000},
		{SenderName: "Me", Body: "ok", TimestampMS: now - 500, IsFromMe: true},
	})
	res := callTool(t, extractActionItemsHandler(a), map[string]any{"conversation_id": "c1"})
	text := resultText(t, res)
	if !strings.Contains(strings.ToLower(text), "can you send") {
		t.Errorf("expected 'can you send' match, got %q", text)
	}
	if !strings.Contains(strings.ToLower(text), "please remember") && !strings.Contains(strings.ToLower(text), "remember to call") {
		t.Errorf("expected remember-to match, got %q", text)
	}
}
