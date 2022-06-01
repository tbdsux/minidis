package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type FollowupContext struct {
	message *discordgo.Message
	event   *discordgo.Interaction
	session *discordgo.Session
	AppID   string
}

// Edit edits the followup message.
func (f *FollowupContext) Edit(content string) error {
	return f.EditC(EditProps{
		Content: content,
	})
}

// EditC is the full edit interaction component structure.
func (f *FollowupContext) EditC(reply EditProps) error {
	res := &discordgo.WebhookEdit{
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
	_, err := f.session.FollowupMessageEdit(f.event, f.message.ID, res)

	return err
}

// Delete deletes the followup message.
func (f *FollowupContext) Delete() error {
	return f.session.FollowupMessageDelete(f.event, f.message.ID)
}
