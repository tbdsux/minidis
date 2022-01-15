package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type InteractionContext struct {
	event   *discordgo.Interaction
	session *discordgo.Session
	AppID   string
}

type InteractionFollowupContext struct {
	message *discordgo.Message
	ic      *InteractionContext
}

func (s *SlashContext) NewInteractionContext() *InteractionContext {
	return &InteractionContext{
		event:   s.event,
		session: s.session,
		AppID:   s.session.State.User.ID,
	}
}

// Edit edis the interaction response.
func (i *InteractionContext) Edit(content string) error {
	return i.EditC(EditProps{
		Content: content,
	})
}

type EditProps struct {
	Content         string
	Embeds          []*discordgo.MessageEmbed
	Components      []discordgo.MessageComponent
	Attachments     []*discordgo.File
	AllowedMentions *discordgo.MessageAllowedMentions
}

// EditC is the full edit interaction component structure.
func (i *InteractionContext) EditC(reply EditProps) error {
	res := &discordgo.WebhookEdit{
		Content: reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// edit interaction response
	_, err := i.session.InteractionResponseEdit(i.AppID, i.event, res)

	return err
}

// Delete deletes the interaction response.
func (i *InteractionContext) Delete() error {
	return i.session.InteractionResponseDelete(i.AppID, i.event)
}

// Followup creates a followup message to the interaction response.
func (i *InteractionContext) Followup(content string) (*InteractionFollowupContext, error) {
	message, err := i.session.FollowupMessageCreate(i.AppID, i.event, true, &discordgo.WebhookParams{
		Content: content,
	})

	if err != nil {
		return nil, err
	}

	context := &InteractionFollowupContext{
		message: message,
		ic:      i,
	}

	return context, nil
}

type FollowupProps ReplyProps

// FollowupC is the full followup component structure.
func (i *InteractionContext) FollowupC(reply FollowupProps) (*InteractionFollowupContext, error) {
	res := &discordgo.WebhookParams{
		Content: reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.IsEphemeral {
		res.Flags = 1 << 6
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// send follup
	message, err := i.session.FollowupMessageCreate(i.AppID, i.event, true, res)
	if err != nil {
		return nil, err
	}

	// return new context
	return &InteractionFollowupContext{
		message: message,
		ic:      i,
	}, nil
}

// Edit edits the followup message.
func (f *InteractionFollowupContext) Edit(content string) error {
	return f.EditC(EditProps{
		Content: content,
	})
}

// EditC is the full edit interaction component structure.
func (f *InteractionFollowupContext) EditC(reply EditProps) error {
	res := &discordgo.WebhookEdit{
		Content: reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// edit followup response
	_, err := f.ic.session.FollowupMessageEdit(f.ic.AppID, f.ic.event, f.message.ID, res)

	return err
}

// Delete deletes the followup message.
func (f *InteractionFollowupContext) Delete() error {
	return f.ic.session.FollowupMessageDelete(f.ic.AppID, f.ic.event, f.message.ID)
}
