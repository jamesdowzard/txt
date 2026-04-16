package client

import (
	"fmt"

	"go.mau.fi/mautrix-gmessages/pkg/libgm/gmproto"
)

// SetConversationArchived toggles the archived state of a conversation on the
// Google Messages server. Returns an error if the RPC fails or if the server
// responds with Success=false. Uses ConversationStatus_ARCHIVED (archived=true)
// or _ACTIVE (archived=false).
func (c *Client) SetConversationArchived(convID string, archived bool) error {
	status := gmproto.ConversationStatus_ACTIVE
	if archived {
		status = gmproto.ConversationStatus_ARCHIVED
	}
	req := &gmproto.UpdateConversationRequest{
		ConversationID: convID,
		Data: &gmproto.UpdateConversationRequest_UpdateData{
			UpdateData: &gmproto.UpdateConversationData{
				ConversationID: convID,
				Data: &gmproto.UpdateConversationData_Status{
					Status: status,
				},
			},
		},
	}
	resp, err := c.GM.UpdateConversation(req)
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	if resp != nil && !resp.GetSuccess() {
		return fmt.Errorf("google messages rejected archive update for %s", convID)
	}
	return nil
}
