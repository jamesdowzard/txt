package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/maxghenis/openmessage/internal/app"
)

func getPersonMessagesTool() mcp.Tool {
	return mcp.NewTool("get_person_messages",
		mcp.WithDescription("Get all messages with a person across all platforms (SMS, Google Chat, iMessage, WhatsApp). Searches by name or identifier."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Person's name to search for (case-insensitive partial match)")),
		mcp.WithNumber("limit", mcp.Description("Maximum messages to return (default 50)")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func getPersonMessagesHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		name := strArg(args, "name")
		if name == "" {
			return errorResult("name is required"), nil
		}
		limit := intArg(args, "limit", 50)

		// Find all conversations that mention this person
		allConvs, err := a.Store.ListConversations(1000)
		if err != nil {
			return errorResult(fmt.Sprintf("list conversations: %v", err)), nil
		}

		nameLower := strings.ToLower(name)
		var matchingConvIDs []string
		for _, c := range allConvs {
			if strings.Contains(strings.ToLower(c.Name), nameLower) ||
				strings.Contains(strings.ToLower(c.Participants), nameLower) {
				matchingConvIDs = append(matchingConvIDs, c.ConversationID)
			}
		}

		if len(matchingConvIDs) == 0 {
			return textResult(fmt.Sprintf("No conversations found with '%s'.", name)), nil
		}

		// Collect messages from matching conversations
		var sb strings.Builder
		sb.WriteString(messagePreamble)
		fmt.Fprintf(&sb, "Messages with '%s' across %d conversation(s):\n\n", name, len(matchingConvIDs))

		totalMsgs := 0
		perConvLimit := limit / len(matchingConvIDs)
		if perConvLimit < 10 {
			perConvLimit = 10
		}

		for _, convID := range matchingConvIDs {
			conv, _ := a.Store.GetConversation(convID)
			if conv == nil {
				continue
			}

			msgs, err := a.Store.GetMessagesByConversation(convID, perConvLimit)
			if err != nil {
				continue
			}

			if len(msgs) == 0 {
				continue
			}

			platform := conv.SourcePlatform
			if platform == "" {
				platform = "sms"
			}
			fmt.Fprintf(&sb, "--- %s [%s] (ID: %s) ---\n", conv.Name, platform, convID)

			for _, m := range msgs {
				ts := time.UnixMilli(m.TimestampMS).Format(time.RFC3339)
				direction := "←"
				if m.IsFromMe {
					direction = "→"
				}
				sender := m.SenderName
				if sender == "" {
					sender = m.SenderNumber
				}
				if sender == "" {
					sender = "Unknown"
				}
				display := formatMessageBody(m.Body, m.MediaID, m.MimeType, m.MessageID)
				fmt.Fprintf(&sb, "[%s] %s %s: «%s»\n", ts, direction, sender, display)
				totalMsgs++
			}
			sb.WriteString("\n")

			if totalMsgs >= limit {
				break
			}
		}

		fmt.Fprintf(&sb, "Total: %d messages across %d conversation(s)\n", totalMsgs, len(matchingConvIDs))
		return textResult(sb.String()), nil
	}
}
