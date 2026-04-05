package notify

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/db"
)

const notificationHistoryCap = 256

type commandRunner func(name string, args ...string) error

type MacOSNotifier struct {
	logger               zerolog.Logger
	enabled              bool
	run                  commandRunner
	baseURL              string
	terminalNotifierPath string

	mu    sync.Mutex
	seen  map[string]struct{}
	order []string
}

func NewMacOSNotifier(logger zerolog.Logger, enabled bool, baseURL string) *MacOSNotifier {
	notifier := &MacOSNotifier{
		logger:  logger,
		enabled: enabled,
		run: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		seen:    make(map[string]struct{}),
	}
	if enabled {
		if path, err := exec.LookPath("terminal-notifier"); err == nil {
			notifier.terminalNotifierPath = path
		}
	}
	return notifier
}

func (n *MacOSNotifier) Enabled() bool {
	return n != nil && n.enabled
}

func (n *MacOSNotifier) NotifyIncomingMessage(message *db.Message) {
	if n == nil || !n.enabled || message == nil || message.IsFromMe {
		return
	}
	messageID := strings.TrimSpace(message.MessageID)
	if messageID == "" || !n.remember(messageID) {
		return
	}

	title := strings.TrimSpace(message.SenderName)
	if title == "" {
		title = strings.TrimSpace(message.SenderNumber)
	}
	if title == "" {
		title = "OpenMessage"
	}

	body := strings.TrimSpace(message.Body)
	if body == "" {
		if message.MediaID != "" || message.MimeType != "" {
			body = "Sent an attachment"
		} else {
			body = "New message"
		}
	}

	go func() {
		if err := n.notify(title, body, messageID, message.ConversationID); err != nil {
			n.logger.Debug().Err(err).Str("msg_id", messageID).Msg("macOS notification failed")
		}
	}()
}

func (n *MacOSNotifier) notify(title, body, messageID, conversationID string) error {
	if n.terminalNotifierPath != "" {
		args := []string{
			"-title", title,
			"-subtitle", "OpenMessage",
			"-message", body,
			"-group", "openmessage:" + messageID,
		}
		if openURL := n.openURL(conversationID); openURL != "" {
			args = append(args, "-open", openURL)
		}
		return n.run(n.terminalNotifierPath, args...)
	}
	return n.run("osascript", "-e", appleScriptNotification(title, body))
}

func (n *MacOSNotifier) openURL(conversationID string) string {
	if n == nil || n.baseURL == "" {
		return ""
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return n.baseURL + "/"
	}
	return n.baseURL + "/?conversation=" + url.QueryEscape(conversationID)
}

func (n *MacOSNotifier) remember(messageID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.seen[messageID]; exists {
		return false
	}
	n.seen[messageID] = struct{}{}
	n.order = append(n.order, messageID)
	if len(n.order) > notificationHistoryCap {
		evicted := n.order[0]
		n.order = n.order[1:]
		delete(n.seen, evicted)
	}
	return true
}

func appleScriptNotification(title, body string) string {
	return fmt.Sprintf(
		"display notification %s with title %s",
		appleScriptString(body),
		appleScriptString(title),
	)
}

func appleScriptString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
