package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jamesdowzard/txt/internal/db"
)

// writeConversationExport serialises a conversation + its (chronologically
// ordered) messages to w in the requested format. Supported formats: json,
// csv, md. Sets Content-Type + Content-Disposition so browsers treat the
// response as a download with a sensible filename.
func writeConversationExport(w http.ResponseWriter, format string, convo *db.Conversation, msgs []*db.Message) error {
	safeName := slugifyConversationName(convo)
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="textbridge-%s.json"`, safeName))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"conversation": convo,
			"messages":     msgs,
			"exported_at":  time.Now().UTC().Format(time.RFC3339),
		})
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="textbridge-%s.csv"`, safeName))
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"message_id", "timestamp_iso", "timestamp_ms", "sender_name", "sender_number", "is_from_me", "source_platform", "body", "media_id", "mime_type"}); err != nil {
			return err
		}
		for _, m := range msgs {
			if err := cw.Write([]string{
				m.MessageID,
				time.UnixMilli(m.TimestampMS).UTC().Format(time.RFC3339),
				fmt.Sprintf("%d", m.TimestampMS),
				m.SenderName,
				m.SenderNumber,
				fmt.Sprintf("%t", m.IsFromMe),
				m.SourcePlatform,
				m.Body,
				m.MediaID,
				m.MimeType,
			}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="textbridge-%s.md"`, safeName))
		return writeMarkdownExport(w, convo, msgs)
	default:
		return fmt.Errorf("invalid export format %q (want json|csv|md)", format)
	}
}

func writeMarkdownExport(w http.ResponseWriter, convo *db.Conversation, msgs []*db.Message) error {
	name := convo.Name
	if convo.Nickname != "" {
		name = convo.Nickname
	}
	if name == "" {
		name = convo.ConversationID
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", name)
	if convo.SourcePlatform != "" && convo.SourcePlatform != "sms" {
		fmt.Fprintf(&sb, "_Platform: %s_\n\n", convo.SourcePlatform)
	}
	fmt.Fprintf(&sb, "_Exported %s (%d messages)_\n\n", time.Now().UTC().Format(time.RFC3339), len(msgs))
	sb.WriteString("---\n\n")

	var lastDay string
	for _, m := range msgs {
		ts := time.UnixMilli(m.TimestampMS).Local()
		day := ts.Format("2006-01-02")
		if day != lastDay {
			fmt.Fprintf(&sb, "\n## %s\n\n", day)
			lastDay = day
		}
		direction := "←"
		sender := m.SenderName
		if m.IsFromMe {
			direction = "→"
			sender = "me"
		}
		if sender == "" {
			sender = m.SenderNumber
		}
		if sender == "" {
			sender = "(unknown)"
		}
		fmt.Fprintf(&sb, "**%s** %s `[%s]`  \n", sender, direction, ts.Format("15:04"))
		body := m.Body
		if body == "" && m.MediaID != "" {
			body = fmt.Sprintf("_[attachment — %s, message_id: %s]_", m.MimeType, m.MessageID)
		}
		body = strings.ReplaceAll(body, "\n", "  \n")
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}
	_, err := w.Write([]byte(sb.String()))
	return err
}

func slugifyConversationName(convo *db.Conversation) string {
	source := convo.Nickname
	if source == "" {
		source = convo.Name
	}
	if source == "" {
		source = convo.ConversationID
	}
	var out strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			out.WriteByte('-')
		}
	}
	if out.Len() == 0 {
		return "conversation"
	}
	s := strings.ToLower(out.String())
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
