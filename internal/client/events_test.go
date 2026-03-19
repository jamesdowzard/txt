package client

import (
	"testing"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-gmessages/pkg/libgm"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/db"
)

func TestHandleMessage_RemovesOnlyMatchingTmpPlaceholder(t *testing.T) {
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.UpsertMessage(&db.Message{
		MessageID:      "tmp_match",
		ConversationID: "c1",
		Body:           "pending 1",
		IsFromMe:       true,
		TimestampMS:    1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMessage(&db.Message{
		MessageID:      "tmp_other",
		ConversationID: "c1",
		Body:           "pending 2",
		IsFromMe:       true,
		TimestampMS:    1001,
	}); err != nil {
		t.Fatal(err)
	}

	handler := &EventHandler{
		Store:  store,
		Logger: zerolog.Nop(),
	}

	handler.handleMessage(&libgm.WrappedMessage{
		Message: &gmproto.Message{
			MessageID:      "real_msg_1",
			ConversationID: "c1",
			Timestamp:      2000 * 1000,
			TmpID:          "tmp_match",
			SenderParticipant: &gmproto.Participant{
				IsMe:     true,
				FullName: "Me",
				ID:       &gmproto.SmallInfo{Number: "+15551234567"},
			},
			MessageInfo: []*gmproto.MessageInfo{{
				Data: &gmproto.MessageInfo_MessageContent{
					MessageContent: &gmproto.MessageContent{Content: "delivered"},
				},
			}},
		},
	})

	if got, err := store.GetMessageByID("tmp_match"); err != nil {
		t.Fatalf("lookup tmp_match: %v", err)
	} else if got != nil {
		t.Fatalf("tmp_match should have been removed, got %+v", got)
	}

	if got, err := store.GetMessageByID("tmp_other"); err != nil {
		t.Fatalf("lookup tmp_other: %v", err)
	} else if got == nil {
		t.Fatal("tmp_other should remain in the store")
	}

	if got, err := store.GetMessageByID("real_msg_1"); err != nil {
		t.Fatalf("lookup real message: %v", err)
	} else if got == nil {
		t.Fatal("real echoed message should be stored")
	}
}
