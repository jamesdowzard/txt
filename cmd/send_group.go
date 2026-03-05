package cmd

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"

	"github.com/maxghenis/openmessage/internal/app"
)

func RunSendGroup(logger zerolog.Logger, phones []string, message string) error {
	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if err := a.LoadAndConnect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	convResp, err := a.Client.GM.GetOrCreateConversation(&gmproto.GetOrCreateConversationRequest{
		Numbers: app.NewContactNumbers(phones),
	})
	if err != nil {
		return fmt.Errorf("get/create group conversation: %w", err)
	}

	conv := convResp.GetConversation()
	if conv == nil {
		return fmt.Errorf("no conversation returned")
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
		return fmt.Errorf("send: %w", err)
	}

	logger.Info().
		Str("conversation", conv.GetConversationID()).
		Str("recipients", strings.Join(phones, ", ")).
		Msg("Group message sent")
	return nil
}
