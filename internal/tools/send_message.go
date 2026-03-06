package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/app"
)

func sendMessageTool() mcp.Tool {
	return mcp.NewTool("send_message",
		mcp.WithDescription("Send a text message (SMS/RCS) to a phone number"),
		mcp.WithString("phone_number", mcp.Required(), mcp.Description("Recipient phone number with country code (e.g., +15551234567)")),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message text to send")),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
	)
}

func sendMessageHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		phone := strArg(args, "phone_number")
		message := strArg(args, "message")

		if phone == "" {
			return errorResult("phone_number is required"), nil
		}
		if message == "" {
			return errorResult("message is required"), nil
		}
		if a.Client == nil {
			return errorResult(app.ErrNotConnected), nil
		}

		convResp, err := a.Client.GM.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
			Numbers: app.NewContactNumbers([]string{phone}),
		})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get/create conversation: %v", err)), nil
		}

		conv := convResp.GetConversation()
		if conv == nil {
			return errorResult("no conversation returned"), nil
		}

		payload := app.BuildSendPayload(conv.GetConversationID(), message, "", "", nil)
		_, err = a.Client.GM.SendMessage(payload)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to send: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Message sent to %s: %s", phone, message)), nil
	}
}
