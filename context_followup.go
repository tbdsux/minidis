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
	res := &discord.WebhookEdit{
		Content: &reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = &reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = &reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// edit followup response
	_, err := f.Session.FollowupMessageEdit(f.Interaction, f.message.ID, res)

	return err
}

// Delete deletes the followup message.
func (f *FollowupContext) Delete() error {
	return f.Session.FollowupMessageDelete(f.Interaction, f.message.ID)
}
