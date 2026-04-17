package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/maxghenis/openmessage/internal/app"
)

func summarizeConversationTool() mcp.Tool {
	return mcp.NewTool("summarize_conversation",
		mcp.WithDescription("Produce a short heuristic summary of a conversation: participant counts, message counts, date range, and the last few message lines. Does NOT call an LLM — purely statistical, safe to run frequently."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation to summarize")),
		mcp.WithNumber("message_limit", mcp.Description("How many recent messages to scan (default 50, max 500)")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func summarizeConversationHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}
		limit := intArg(args, "message_limit", 50)
		if limit < 1 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}
		convo, err := a.Store.GetConversation(convID)
		if err != nil {
			return errorResult(fmt.Sprintf("load conversation: %v", err)), nil
		}
		if convo == nil {
			return errorResult("conversation not found"), nil
		}
		msgs, err := a.Store.GetMessagesByConversation(convID, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("load messages: %v", err)), nil
		}
		if len(msgs) == 0 {
			return textResult(messagePreamble + fmt.Sprintf("Conversation %q (%s) has no messages in the local store.", convo.Name, convID)), nil
		}

		// Messages from Store are newest-first — flip for chronological display.
		chrono := make([]int, len(msgs))
		for i := range msgs {
			chrono[i] = len(msgs) - 1 - i
		}

		senders := map[string]int{}
		var inbound, outbound int
		var earliest, latest int64
		for _, m := range msgs {
			senders[resolveSender(m)]++
			if m.IsFromMe {
				outbound++
			} else {
				inbound++
			}
			if earliest == 0 || m.TimestampMS < earliest {
				earliest = m.TimestampMS
			}
			if m.TimestampMS > latest {
				latest = m.TimestampMS
			}
		}

		type kv struct {
			name  string
			count int
		}
		ordered := make([]kv, 0, len(senders))
		for n, c := range senders {
			ordered = append(ordered, kv{n, c})
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].count > ordered[j].count })

		var sb strings.Builder
		sb.WriteString(messagePreamble)
		fmt.Fprintf(&sb, "Conversation: %s", convo.Name)
		if convo.IsGroup {
			sb.WriteString(" [group]")
		}
		if convo.SourcePlatform != "" && convo.SourcePlatform != "sms" {
			fmt.Fprintf(&sb, " [%s]", convo.SourcePlatform)
		}
		fmt.Fprintf(&sb, "\nID: %s\n", convID)
		fmt.Fprintf(&sb, "Window: %s → %s (%d messages)\n",
			time.UnixMilli(earliest).Format(time.RFC3339),
			time.UnixMilli(latest).Format(time.RFC3339),
			len(msgs),
		)
		fmt.Fprintf(&sb, "Direction: %d inbound / %d outbound\n", inbound, outbound)
		sb.WriteString("Top senders: ")
		for i, kv := range ordered {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%s (%d)", kv.name, kv.count)
			if i >= 4 {
				break
			}
		}
		sb.WriteString("\n\nRecent messages (chronological):\n")
		tail := 10
		if len(chrono) < tail {
			tail = len(chrono)
		}
		for _, idx := range chrono[len(chrono)-tail:] {
			sb.WriteString(formatMessageLine(msgs[idx]))
			sb.WriteByte('\n')
		}
		return textResult(sb.String()), nil
	}
}
