package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/jamesdowzard/txt/internal/app"
)

func findUnansweredTool() mcp.Tool {
	return mcp.NewTool("find_unanswered",
		mcp.WithDescription("Scan recent conversations and return the ones whose last message is inbound (from the other side) within the lookback window. Useful for 'who's waiting on me?'."),
		mcp.WithNumber("scan_limit", mcp.Description("How many recent conversations to scan (default 50, max 500)")),
		mcp.WithNumber("max_age_hours", mcp.Description("Only include conversations whose last message is newer than this many hours (default 168 = 7 days)")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func findUnansweredHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		scan := intArg(args, "scan_limit", 50)
		if scan < 1 {
			scan = 50
		}
		if scan > 500 {
			scan = 500
		}
		maxAge := intArg(args, "max_age_hours", 168)
		if maxAge < 1 {
			maxAge = 168
		}
		cutoff := time.Now().Add(-time.Duration(maxAge) * time.Hour).UnixMilli()

		convs, err := a.Store.ListConversations(scan)
		if err != nil {
			return errorResult(fmt.Sprintf("list conversations: %v", err)), nil
		}

		type hit struct {
			name    string
			convID  string
			ts      int64
			preview string
			sender  string
		}
		var hits []hit
		for _, c := range convs {
			if c.LastMessageTS < cutoff {
				continue
			}
			msgs, err := a.Store.GetMessagesByConversation(c.ConversationID, 1)
			if err != nil || len(msgs) == 0 {
				continue
			}
			last := msgs[0]
			if last.IsFromMe {
				continue
			}
			preview := last.Body
			if preview == "" && last.MediaID != "" {
				preview = "[attachment]"
			}
			if n := 80; len(preview) > n {
				preview = preview[:n] + "…"
			}
			hits = append(hits, hit{
				name:    c.Name,
				convID:  c.ConversationID,
				ts:      last.TimestampMS,
				preview: preview,
				sender:  resolveSender(last),
			})
		}
		if len(hits) == 0 {
			return textResult(fmt.Sprintf("No unanswered inbound threads in the last %dh.", maxAge)), nil
		}
		var sb strings.Builder
		sb.WriteString(messagePreamble)
		fmt.Fprintf(&sb, "%d unanswered threads (last %dh):\n\n", len(hits), maxAge)
		for _, h := range hits {
			fmt.Fprintf(&sb, "- %s (ID: %s)\n    ← %s at %s: «%s»\n",
				h.name, h.convID, h.sender,
				time.UnixMilli(h.ts).Format(time.RFC3339),
				h.preview,
			)
		}
		return textResult(sb.String()), nil
	}
}
