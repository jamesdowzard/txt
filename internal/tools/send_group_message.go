package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/app"
)

func sendGroupMessageTool() mcp.Tool {
	return mcp.NewTool("send_group_message",
		mcp.WithDescription("Send a text message to a group conversation (MMS group). Creates the group if it doesn't exist."),
		mcp.WithString("phone_numbers", mcp.Required(), mcp.Description(`JSON array of phone numbers with country code, e.g. ["+15551234567", "+15559876543"]`)),
		mcp.WithString("message", mcp.Required(), mcp.Description("Message text to send")),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
	)
}

func sendGroupMessageHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		phonesRaw := strArg(args, "phone_numbers")
		message := strArg(args, "message")

		if phonesRaw == "" {
			return errorResult("phone_numbers is required"), nil
		}
		if message == "" {
			return errorResult("message is required"), nil
		}

		var phones []string
		if err := json.Unmarshal([]byte(phonesRaw), &phones); err != nil {
			return errorResult(fmt.Sprintf("phone_numbers must be a JSON array of strings: %v", err)), nil
		}
		if len(phones) < 2 {
			return errorResult("phone_numbers must contain at least 2 numbers for a group message"), nil
		}

		if a.Client == nil {
			return errorResult("not connected to Google Messages"), nil
		}

		numbers := make([]*gmproto.ContactNumber, len(phones))
		for i, phone := range phones {
			numbers[i] = &gmproto.ContactNumber{
				MysteriousInt: 7,
				Number:        phone,
				Number2:       phone,
			}
		}

		convResp, err := a.Client.GM.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
			Numbers: numbers,
		})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to get/create group conversation: %v", err)), nil
		}

		conv := convResp.GetConversation()
		if conv == nil {
			return errorResult("no conversation returned"), nil
		}

		tmpID := uuid.NewString()
		_, err = a.Client.GM.SendMessage(&gmproto.SendMessageRequest{
			ConversationID: conv.GetConversationID(),
			TmpID:          tmpID,
			MessagePayload: &gmproto.MessagePayload{
				TmpID:          tmpID,
				TmpID2:         tmpID,
				ConversationID: conv.GetConversationID(),
				ParticipantID:  conv.GetDefaultOutgoingID(),
				MessageInfo: []*gmproto.MessageInfo{
					{
						Data: &gmproto.MessageInfo_MessageContent{
							MessageContent: &gmproto.MessageContent{
								Content: message,
							},
						},
					},
				},
			},
		})
		if err != nil {
			return errorResult(fmt.Sprintf("failed to send group message: %v", err)), nil
		}

		return textResult(fmt.Sprintf("Group message sent to %s: %s", strings.Join(phones, ", "), message)), nil
	}
}
