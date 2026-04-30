package cmd

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/internal/app"
)

func RunSend(logger zerolog.Logger, conversationID, message string) error {
	if os.Getenv("OPENMESSAGES_ALLOW_DIRECT_SEND") != "1" {
		return fmt.Errorf("direct send is disabled by default; use `openmessage draft` then `openmessage send-draft --confirm <draft_id>`, or set OPENMESSAGES_ALLOW_DIRECT_SEND=1 to opt into unsafe direct sends")
	}

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
	cli := a.GetClient()
	if cli == nil {
		return fmt.Errorf("client not connected")
	}
	_, err = cli.GM.SendMessage(payload)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}

	logger.Info().Str("conversation", conversationID).Msg("Message sent")
	return nil
}
