package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/jamesdowzard/txt/internal/app"
)

// actionItemPatterns fires on sentences that look like imperatives or requests.
// Kept intentionally narrow — false positives from broad patterns (e.g., "I")
// are worse than misses since the caller sees every hit in the output.
var actionItemPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:can|could|would|will) you\b[^?.!\n]{0,120}[?.!]?`),
	regexp.MustCompile(`(?i)\bplease\b[^.!?\n]{2,120}[.!?]?`),
	regexp.MustCompile(`(?i)\bremember to\b[^.!?\n]{2,120}[.!?]?`),
	regexp.MustCompile(`(?i)\blet (?:me|us) know\b[^.!?\n]{0,120}[.!?]?`),
	regexp.MustCompile(`(?i)\bneed (?:to|you to)\b[^.!?\n]{2,120}[.!?]?`),
	regexp.MustCompile(`(?i)\bdon't forget\b[^.!?\n]{2,120}[.!?]?`),
	regexp.MustCompile(`(?i)\bmake sure\b[^.!?\n]{2,120}[.!?]?`),
}

func extractActionItemsTool() mcp.Tool {
	return mcp.NewTool("extract_action_items",
		mcp.WithDescription("Scan recent messages in a conversation for imperative / request phrases ('can you…', 'please…', 'let me know…', 'don't forget…') and return a deduplicated list. Heuristic regex match — not an LLM. Useful as a starting point before handing off to a stronger summariser."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation to scan")),
		mcp.WithNumber("message_limit", mcp.Description("How many recent messages to scan (default 100, max 500)")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func extractActionItemsHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}
		limit := intArg(args, "message_limit", 100)
		if limit < 1 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}
		msgs, err := a.Store.GetMessagesByConversation(convID, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("load messages: %v", err)), nil
		}
		if len(msgs) == 0 {
			return textResult("No messages in conversation."), nil
		}

		type hit struct {
			sender string
			ts     int64
			text   string
		}
		seen := map[string]struct{}{}
		var hits []hit
		// Walk newest-first so the output lists most recent asks on top.
		for _, m := range msgs {
			body := m.Body
			if body == "" {
				continue
			}
			for _, pat := range actionItemPatterns {
				for _, match := range pat.FindAllString(body, -1) {
					trimmed := strings.TrimSpace(match)
					key := strings.ToLower(trimmed)
					if key == "" {
						continue
					}
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					hits = append(hits, hit{
						sender: resolveSender(m),
						ts:     m.TimestampMS,
						text:   trimmed,
					})
				}
			}
		}
		if len(hits) == 0 {
			return textResult("No action-item patterns found in recent messages."), nil
		}
		var sb strings.Builder
		sb.WriteString(messagePreamble)
		fmt.Fprintf(&sb, "%d action-item candidates:\n\n", len(hits))
		for _, h := range hits {
			fmt.Fprintf(&sb, "- [%s] %s: %s\n",
				time.UnixMilli(h.ts).Format(time.RFC3339),
				h.sender,
				h.text,
			)
		}
		return textResult(sb.String()), nil
	}
}
