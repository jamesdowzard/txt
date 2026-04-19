package web

import (
	"strings"
	"testing"

	"github.com/jamesdowzard/txt/internal/db"
)

func TestPickIMessageBuddy(t *testing.T) {
	cases := []struct {
		name        string
		conv        *db.Conversation
		wantBuddy   string
		wantService string
		wantErr     string
	}{
		{
			name: "single phone participant",
			conv: &db.Conversation{
				Participants: `[{"name":"Tommi Yick","number":"+61437590462"}]`,
			},
			wantBuddy:   "+61437590462",
			wantService: "iMessage",
		},
		{
			name: "email participant",
			conv: &db.Conversation{
				Participants: `[{"name":"Tommi Yick","number":"tommi@me.com"}]`,
			},
			wantBuddy:   "tommi@me.com",
			wantService: "iMessage",
		},
		{
			name: "SMS chat routes via SMS service",
			conv: &db.Conversation{
				ConversationID: "imessage:SMS;-;+61437590462",
				Participants:   `[{"number":"+61437590462"}]`,
			},
			wantBuddy:   "+61437590462",
			wantService: "SMS",
		},
		{
			name: "iMessage prefix keeps iMessage service",
			conv: &db.Conversation{
				ConversationID: "imessage:iMessage;-;+61437590462",
				Participants:   `[{"number":"+61437590462"}]`,
			},
			wantBuddy:   "+61437590462",
			wantService: "iMessage",
		},
		{
			name: "group blocked via pickIMessageBuddy (use pickIMessageTarget)",
			conv: &db.Conversation{
				IsGroup:        true,
				ConversationID: "imessage:any;+;chat123",
				Participants:   `[{"number":"+1"}]`,
			},
			wantErr: "group iMessage send not supported via pickIMessageBuddy",
		},
		{
			name:    "no participants",
			conv:    &db.Conversation{Participants: `[]`},
			wantErr: "no participant handle found",
		},
		{
			name:    "bad json",
			conv:    &db.Conversation{Participants: `not-json`},
			wantErr: "parse participants",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buddy, service, err := pickIMessageBuddy(c.conv)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if buddy != c.wantBuddy {
				t.Errorf("buddy = %q, want %q", buddy, c.wantBuddy)
			}
			if service != c.wantService {
				t.Errorf("service = %q, want %q", service, c.wantService)
			}
		})
	}
}

func TestBuildReplyAppleScript(t *testing.T) {
	cases := []struct {
		name     string
		target   imessageTarget
		message  string
		wantSubs []string
	}{
		{
			name: "1:1 reply resolves chat via participant",
			target: imessageTarget{
				buddy:       "+61437590462",
				service:     "iMessage",
				replyToGUID: "abc-123",
			},
			message: "sure",
			wantSubs: []string{
				`service type = iMessage`,
				`buddy "+61437590462"`,
				`first chat whose participants contains targetBuddy`,
				`send "sure" to message id "abc-123" of targetChat`,
			},
		},
		{
			name: "group reply addresses chat id directly",
			target: imessageTarget{
				chatGUID:    "iMessage;+;chat54023337152810558",
				replyToGUID: "def-456",
			},
			message: "agreed",
			wantSubs: []string{
				`set targetChat to chat id "iMessage;+;chat54023337152810558"`,
				`send "agreed" to message id "def-456" of targetChat`,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildReplyAppleScript(c.target, c.message)
			for _, sub := range c.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("script missing %q\n---\n%s", sub, got)
				}
			}
		})
	}
}

func TestPickIMessageTarget(t *testing.T) {
	cases := []struct {
		name     string
		conv     *db.Conversation
		wantChat string
		wantBud  string
		wantErr  string
	}{
		{
			name: "1:1 phone",
			conv: &db.Conversation{
				Participants: `[{"number":"+61437590462"}]`,
			},
			wantBud: "+61437590462",
		},
		{
			name: "group with any;+ prefix gets normalised to iMessage;+",
			conv: &db.Conversation{
				IsGroup:        true,
				ConversationID: "imessage:any;+;chat54023337152810558",
				Participants:   `[{"number":"+61400000001"},{"number":"+61400000002"}]`,
			},
			wantChat: "iMessage;+;chat54023337152810558",
		},
		{
			name: "group already prefixed with iMessage;+",
			conv: &db.Conversation{
				IsGroup:        true,
				ConversationID: "imessage:iMessage;+;chat999",
			},
			wantChat: "iMessage;+;chat999",
		},
		{
			name:    "group without chat GUID rejected",
			conv:    &db.Conversation{IsGroup: true, ConversationID: "imessage:"},
			wantErr: "group conversation has no chat GUID",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tg, err := pickIMessageTarget(c.conv)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if tg.chatGUID != c.wantChat {
				t.Errorf("chatGUID = %q, want %q", tg.chatGUID, c.wantChat)
			}
			if tg.buddy != c.wantBud {
				t.Errorf("buddy = %q, want %q", tg.buddy, c.wantBud)
			}
		})
	}
}
