package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/db"
)

func TestMacOSNotifierDedupesByMessageID(t *testing.T) {
	notifier := NewMacOSNotifier(zerolog.Nop(), true, "")

	calls := make(chan string, 2)
	notifier.run = func(name string, args ...string) error {
		calls <- name
		return nil
	}

	message := &db.Message{
		MessageID:      "m1",
		SenderName:     "Alice",
		Body:           "Hello",
		TimestampMS:    100,
		IsFromMe:       false,
		ConversationID: "c1",
	}

	notifier.NotifyIncomingMessage(message)
	notifier.NotifyIncomingMessage(message)

	select {
	case <-calls:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected notification command to run once")
	}

	select {
	case extra := <-calls:
		t.Fatalf("unexpected duplicate notification command: %q", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestMacOSNotifierSkipsOutgoingMessages(t *testing.T) {
	notifier := NewMacOSNotifier(zerolog.Nop(), true, "")

	called := false
	notifier.run = func(name string, args ...string) error {
		called = true
		return nil
	}

	notifier.NotifyIncomingMessage(&db.Message{
		MessageID: "m1",
		Body:      "sent",
		IsFromMe:  true,
	})

	if called {
		t.Fatal("notifier should not run for outgoing messages")
	}
}

func TestAppleScriptNotificationEscapesQuotes(t *testing.T) {
	script := appleScriptNotification(`Jenn "J"`, `He said "hi"`)
	want := `display notification "He said \"hi\"" with title "Jenn \"J\"" subtitle "OpenMessage"`
	if script != want {
		t.Fatalf("script = %q, want %q", script, want)
	}
}

func TestMacOSNotifierUsesTerminalNotifierDeepLink(t *testing.T) {
	notifier := NewMacOSNotifier(zerolog.Nop(), true, "http://127.0.0.1:7007")
	notifier.terminalNotifierPath = "/opt/homebrew/bin/terminal-notifier"

	type call struct {
		name string
		args []string
	}
	calls := make(chan call, 1)
	notifier.run = func(name string, args ...string) error {
		calls <- call{name: name, args: args}
		return nil
	}

	notifier.NotifyIncomingMessage(&db.Message{
		MessageID:      "m1",
		ConversationID: "conv-1",
		SenderName:     "Alice",
		Body:           "Hello",
	})

	var got call
	select {
	case got = <-calls:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected notification command to run")
	}

	if got.name != "/opt/homebrew/bin/terminal-notifier" {
		t.Fatalf("command name = %q, want terminal-notifier path", got.name)
	}

	joined := strings.Join(got.args, "\n")
	if !strings.Contains(joined, "-open\nhttp://127.0.0.1:7007/?conversation=conv-1") {
		t.Fatalf("terminal-notifier args = %q, want -open conversation URL", joined)
	}
}
