package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/maxghenis/openmessage/internal/app"
)

func sendToConversationTool() mcp.Tool {
	return mcp.NewTool("send_to_conversation",
		mcp.WithDescription("Send a text message to an existing conversation by conversation ID"),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Existing conversation ID from list_conversations or get_conversation")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message text to send")),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
	)
}

func sendToConversationHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		conversationID := strArg(args, "conversation_id")
		message := strArg(args, "message")

		if conversationID == "" {
			return errorResult("conversation_id is required"), nil
		}
		if message == "" {
			return errorResult("message is required"), nil
		}

		conv, err := a.Store.GetConversation(conversationID)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to load conversation: %v", err)), nil
		}
		if conv == nil {
			return errorResult(fmt.Sprintf("conversation %s not found", conversationID)), nil
		}

		if conv.SourcePlatform == "whatsapp" {
			msg, err := a.SendWhatsAppText(conversationID, message, "")
			if err != nil {
				return errorResult(fmt.Sprintf("failed to send: %v", err)), nil
			}
			if err := a.Store.RecordOutgoingMessage(msg, ""); err != nil {
				return errorResult(fmt.Sprintf("failed to persist sent message: %v", err)), nil
			}
			return textResult(fmt.Sprintf("Message sent to %s (%s): %s", conv.Name, conversationID, message)), nil
		}

		cli := a.GetClient()
		if cli == nil {
			return errorResult(app.ErrNotConnected), nil
		}

		payload := app.BuildSendPayload(conversationID, message, "", "", nil)
		_, err = cli.GM.SendMessage(payload)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to send: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Message sent to %s (%s): %s", conv.Name, conversationID, message)), nil
	}
}
