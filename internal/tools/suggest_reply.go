package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/jamesdowzard/txt/internal/app"
)

func suggestReplyTool() mcp.Tool {
	return mcp.NewTool("suggest_reply",
		mcp.WithDescription("Suggest a short reply template for the last inbound message in a conversation. Heuristic only (keyword-based) — not an LLM. Returns a handful of candidate replies the caller can pick from or use as priming for a downstream LLM."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation to suggest a reply for")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func suggestReplyHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}
		msgs, err := a.Store.GetMessagesByConversation(convID, 10)
		if err != nil {
			return errorResult(fmt.Sprintf("load messages: %v", err)), nil
		}
		if len(msgs) == 0 {
			return errorResult("conversation has no messages"), nil
		}
		// Newest first — walk until we find an inbound message.
		var target string
		for _, m := range msgs {
			if !m.IsFromMe && strings.TrimSpace(m.Body) != "" {
				target = m.Body
				break
			}
		}
		if target == "" {
			return textResult("No recent inbound text to reply to (only outbound or media messages)."), nil
		}

		lower := strings.ToLower(target)
		var suggestions []string
		switch {
		case containsAny(lower, "?", "can you", "could you", "would you", "do you"):
			suggestions = []string{
				"Good question — let me check and get back to you.",
				"Yes, that works for me.",
				"Hmm, not sure — can you share more detail?",
			}
		case containsAny(lower, "thanks", "thank you", "ty"):
			suggestions = []string{
				"You're welcome!",
				"Anytime 🙂",
				"No worries at all.",
			}
		case containsAny(lower, "sorry", "apologies"):
			suggestions = []string{
				"No worries, these things happen.",
				"All good — appreciate the heads up.",
				"Thanks for letting me know.",
			}
		case containsAny(lower, "when ", "what time", "schedule", "available"):
			suggestions = []string{
				"I'm free later today — what works for you?",
				"Give me a few options and I'll pick one that fits.",
				"Does tomorrow work?",
			}
		default:
			suggestions = []string{
				"Got it, thanks!",
				"Makes sense — I'll follow up shortly.",
				"Noted.",
			}
		}

		var sb strings.Builder
		sb.WriteString(messagePreamble)
		sb.WriteString("Heuristic reply suggestions (pick one; treat as template, not verbatim):\n\n")
		for i, s := range suggestions {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, s)
		}
		return textResult(sb.String()), nil
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
