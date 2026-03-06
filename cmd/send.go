package cmd

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/app"
)

func RunSend(logger zerolog.Logger, conversationID, message string) error {
	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if err := a.LoadAndConnect(); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Look up conversation to get outgoing participant ID
	conv, err := a.Store.GetConversation(conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return fmt.Errorf("conversation %s not found", conversationID)
	}

	payload := app.BuildSendPayload(conversationID, message, "", "", nil)
	_, err = a.Client.GM.SendMessage(payload)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}

	logger.Info().Str("conversation", conversationID).Msg("Message sent")
	return nil
}
