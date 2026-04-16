package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/db"
)

func archiveConversationTool() mcp.Tool {
	return mcp.NewTool("archive_conversation",
		mcp.WithDescription("Archive or unarchive a Google Messages conversation. Syncs with the Messages server and updates local folder state."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation ID as returned by list_conversations.")),
		mcp.WithString("action", mcp.Required(), mcp.Description(`"archive" or "unarchive".`)),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)
}

func archiveConversationHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		action := strArg(args, "action")

		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}
		var archived bool
		var targetFolder string
		switch action {
		case "archive":
			archived = true
			targetFolder = db.FolderArchive
		case "unarchive":
			archived = false
			targetFolder = db.FolderInbox
		default:
			return errorResult(`action must be "archive" or "unarchive"`), nil
		}

		cli := a.GetClient()
		if cli == nil {
			return errorResult(app.ErrNotConnected), nil
		}
		if err := cli.SetConversationArchived(convID, archived); err != nil {
			return errorResult(fmt.Sprintf("libgm archive failed: %v", err)), nil
		}
		if err := a.Store.SetConversationFolder(convID, targetFolder); err != nil {
			return errorResult(fmt.Sprintf("set folder failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Conversation %s → %s", convID, targetFolder)), nil
	}
}
