package web

import (
	"strings"
	"testing"

	"github.com/maxghenis/openmessage/internal/db"
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
			name:    "group blocked",
			conv:    &db.Conversation{IsGroup: true, Participants: `[{"number":"+1"}]`},
			wantErr: "group iMessage send not supported yet",
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
