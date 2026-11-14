package minidis

import (
	"context"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// FollowupMessage is a message sent with Context.Followup. Like the response
// methods of Context, its methods keep working after the handler returns.
type FollowupMessage struct {
	Message *discord.Message

	ctx         context.Context
	session     *client.Session
	interaction *discord.Interaction
}

// Edit replaces the text of the followup message.
func (f *FollowupMessage) Edit(content string) error {
	return f.EditWith(MessageEdit{Content: &content})
}

// EditWith changes the followup message.
func (f *FollowupMessage) EditWith(edit MessageEdit) error {
	_, err := f.session.FollowupMessageEdit(f.ctx, f.interaction, f.Message.ID, edit.webhookEdit())
	return err
}

// Delete deletes the followup message.
func (f *FollowupMessage) Delete() error {
	return f.session.FollowupMessageDelete(f.ctx, f.interaction, f.Message.ID)
}
