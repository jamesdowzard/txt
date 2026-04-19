package web

import (
	"strings"
	"testing"
)

func TestBuildSendMediaAppleScript_1to1(t *testing.T) {
	s := buildSendMediaAppleScript(imessageTarget{buddy: "+61437590462", service: "iMessage"}, "/tmp/foo.png")
	if !strings.Contains(s, `POSIX file "/tmp/foo.png"`) {
		t.Errorf("missing POSIX file: %s", s)
	}
	if !strings.Contains(s, `buddy "+61437590462"`) {
		t.Errorf("missing buddy: %s", s)
	}
	if !strings.Contains(s, "service type = iMessage") {
		t.Errorf("missing service: %s", s)
	}
}

func TestBuildSendMediaAppleScript_Group(t *testing.T) {
	s := buildSendMediaAppleScript(imessageTarget{chatGUID: "iMessage;+;chat123"}, "/tmp/foo.png")
	if !strings.Contains(s, `to chat id "iMessage;+;chat123"`) {
		t.Errorf("missing chat id: %s", s)
	}
	if !strings.Contains(s, `POSIX file "/tmp/foo.png"`) {
		t.Errorf("missing POSIX file: %s", s)
	}
}
