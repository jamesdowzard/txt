package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/story"
)

func conversationStatsTool() mcp.Tool {
	return mcp.NewTool("conversation_stats",
		mcp.WithDescription("Compute statistics for a conversation: message volume, heatmap, top phrases, response times, gaps. Works with any platform."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("The conversation ID")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func conversationStatsHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}

		// Get all messages for this conversation
		msgs, err := a.Store.GetMessagesByConversation(convID, 100000)
		if err != nil {
			return errorResult(fmt.Sprintf("get messages: %v", err)), nil
		}
		if len(msgs) == 0 {
			return textResult("No messages found in this conversation."), nil
		}

		stats := story.ComputeStats(msgs)
		statsJSON, _ := json.MarshalIndent(stats, "", "  ")

		var sb strings.Builder
		conv, _ := a.Store.GetConversation(convID)
		if conv != nil {
			platform := conv.SourcePlatform
			if platform == "" {
				platform = "sms"
			}
			fmt.Fprintf(&sb, "Stats for: %s [%s]\n\n", conv.Name, platform)
		}

		fmt.Fprintf(&sb, "Total messages: %d\n", stats.TotalMessages)
		fmt.Fprintf(&sb, "Date range: %s to %s\n", stats.DateRange.Start, stats.DateRange.End)
		fmt.Fprintf(&sb, "Sender split: ")
		for sender, count := range stats.SenderSplit {
			fmt.Fprintf(&sb, "%s=%d ", sender, count)
		}
		sb.WriteString("\n")

		if stats.LongestGap.Days > 0 {
			fmt.Fprintf(&sb, "Longest gap: %d days (%s to %s)\n", stats.LongestGap.Days, stats.LongestGap.Start, stats.LongestGap.End)
		}

		for sender, rt := range stats.AvgResponseTimes {
			fmt.Fprintf(&sb, "Avg response time (%s): %d min\n", sender, rt)
		}

		if len(stats.Yearly) > 0 {
			sb.WriteString("\nYearly breakdown:\n")
			for _, ys := range stats.Yearly {
				fmt.Fprintf(&sb, "  %s: %d messages\n", ys.Year, ys.Total)
			}
		}

		if len(stats.TopPhrases) > 0 {
			sb.WriteString("\nTop phrases:\n")
			limit := 20
			if len(stats.TopPhrases) < limit {
				limit = len(stats.TopPhrases)
			}
			for _, p := range stats.TopPhrases[:limit] {
				fmt.Fprintf(&sb, "  %q (%d)\n", p.Phrase, p.Count)
			}
		}

		sb.WriteString("\nFull stats JSON:\n")
		sb.Write(statsJSON)

		return textResult(sb.String()), nil
	}
}

func generateStoryTool() mcp.Tool {
	return mcp.NewTool("generate_story",
		mcp.WithDescription("Generate a narrative story about a conversation or relationship. Creates chapters with themes, quotes, and emotional arc."),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("The conversation ID")),
		mcp.WithString("style", mcp.Description("Story style: intimate, professional, friendship (default: auto-detect)")),
		mcp.WithString("api_key", mcp.Description("Anthropic API key for AI-generated narrative (without it, generates stats + sampled quotes)")),
		mcp.WithDestructiveHintAnnotation(false),
	)
}

func generateStoryHandler(a *app.App) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		convID := strArg(args, "conversation_id")
		if convID == "" {
			return errorResult("conversation_id is required"), nil
		}
		style := strArg(args, "style")
		apiKey := strArg(args, "api_key")

		msgs, err := a.Store.GetMessagesByConversation(convID, 100000)
		if err != nil {
			return errorResult(fmt.Sprintf("get messages: %v", err)), nil
		}
		if len(msgs) == 0 {
			return textResult("No messages found in this conversation."), nil
		}

		s, err := story.Generate(msgs, story.GenerateConfig{
			Style:             style,
			APIKey:            apiKey,
			MaxSampleMessages: 200,
		})
		if err != nil {
			return errorResult(fmt.Sprintf("generate story: %v", err)), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "# %s\n\n", s.Title)
		fmt.Fprintf(&sb, "%s\n\n", s.Summary)
		fmt.Fprintf(&sb, "---\n\n")

		for _, ch := range s.Chapters {
			fmt.Fprintf(&sb, "## %s", ch.Title)
			if ch.Period != "" {
				fmt.Fprintf(&sb, " (%s)", ch.Period)
			}
			sb.WriteString("\n\n")

			if ch.Content != "" {
				fmt.Fprintf(&sb, "%s\n\n", ch.Content)
			}

			for _, q := range ch.Quotes {
				ts := q.Timestamp
				if t, err := time.Parse(time.RFC3339, ts); err == nil {
					ts = t.Format("Jan 2, 2006")
				}
				fmt.Fprintf(&sb, "> **%s** (%s): \"%s\"\n\n", q.Sender, ts, q.Text)
			}
		}

		// Append key stats
		fmt.Fprintf(&sb, "---\n\n**Stats:** %d messages, %s to %s\n",
			s.Stats.TotalMessages, s.Stats.DateRange.Start, s.Stats.DateRange.End)

		return textResult(sb.String()), nil
	}
}
