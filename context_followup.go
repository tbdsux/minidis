package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

type FollowupContext struct {
	message     *discord.Message
	Interaction *discord.Interaction
	Session     *client.Session
	AppID       string
}

// Edit edits the followup message.
func (f *FollowupContext) Edit(content string) error {
	return f.EditC(EditProps{
		Content: content,
	})
}

// EditC is the full edit interaction component structure.
func (f *FollowupContext) EditC(reply EditProps) error {
	_, err := f.Session.FollowupMessageEdit(f.Interaction, f.message.ID, reply.webhookEdit())

	return err
}

// Delete deletes the followup message.
func (f *FollowupContext) Delete() error {
	return f.Session.FollowupMessageDelete(f.Interaction, f.message.ID)
}
