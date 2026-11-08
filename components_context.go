package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

type ComponentContext struct {
	Interaction *discord.Interaction
	Session     *client.Session
	Data        discord.MessageComponentInteractionData

	// Params holds the captures of a handler ID such as "vote:{id}".
	Params map[string]string
}

func (m *Minidis) NewComponentContext(session *client.Session, event *discord.Interaction) *ComponentContext {
	return &ComponentContext{
		Interaction: event,
		Session:     session,
		Data:        event.MessageComponentData(),
	}
}

// Update responds by changing the text of the message the component is on.
func (c *ComponentContext) Update(content string) error {
	return c.UpdateC(EditProps{Content: content})
}

// UpdateC responds by changing the message the component is on. Empty Content
// and nil Embeds or Components are left unchanged; an empty non-nil slice
// clears them, for example to remove the buttons.
func (c *ComponentContext) UpdateC(update EditProps) error {
	return c.Session.InteractionRespond(c.Interaction, &discord.InteractionResponse{
		Type: discord.InteractionResponseUpdateMessage,
		Data: &discord.InteractionResponseData{
			Content:         update.Content,
			Embeds:          update.Embeds,
			Components:      update.Components,
			Files:           update.Attachments,
			AllowedMentions: update.AllowedMentions,
		},
	})
}

// DeferUpdate acknowledges the interaction without showing a loading state on
// a new message. Change the message later with SlashContext.Edit.
func (c *ComponentContext) DeferUpdate() error {
	return c.Session.InteractionRespond(c.Interaction, &discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredMessageUpdate,
	})
}
