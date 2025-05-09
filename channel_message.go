package minidis

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// INFO: Too much abstraction? Maybe, will see if this is useful in the future.
type ChannelMessageContext struct {
	ChannelID string
	MessageID string
	Session   *discordgo.Session
	Message   *discordgo.MessageCreate
}

func (m *Minidis) handleOnMessageCreate(s *discordgo.Session, i *discordgo.MessageCreate) {
	// Check if we have the handler set for the channel
	if handler, ok := m.ChannelMessageHandlers[i.ChannelID]; ok {
		// If the message is from bot, ignore it
		if i.Author.ID == s.State.User.ID {
			return
		}

		ctx := &ChannelMessageContext{
			ChannelID: i.ChannelID,
			MessageID: i.ID,
			Session:   s,
			Message:   i,
		}

		// Call the handler
		if err := handler(ctx); err != nil {
			// TODO: add extra error handler
			log.Printf("failed to execute channel message handler for %s: %v\n", i.ChannelID, err)
		}
		return
	}

	if m.MessageCreateHandler != nil {
		m.MessageCreateHandler(s, i)
	}
}

func (m *Minidis) OnChannelMessageCreate(handler func(c *ChannelMessageContext) error, channels ...string) {
	if m.ChannelMessageHandlers == nil {
		m.ChannelMessageHandlers = make(map[string]func(*ChannelMessageContext) error)
	}

	// Could be useful for a bot in multiple different discord servers and channels
	for _, channel := range channels {
		m.ChannelMessageHandlers[channel] = handler
	}
}

// Reply is a wrapper for sending a message to the channel where the message was sent.
// Setting `doReply` to true will make the message a reply to the original message.
func (c *ChannelMessageContext) Reply(message string, doReply ...bool) (*discordgo.Message, error) {
	failIfNotExists := false

	reply := false
	if len(doReply) > 0 {
		reply = doReply[0]
	}

	if reply {
		return c.Session.ChannelMessageSendReply(c.ChannelID, message, &discordgo.MessageReference{
			MessageID:       c.Message.ID,
			ChannelID:       c.ChannelID,
			GuildID:         c.Message.GuildID,
			FailIfNotExists: &failIfNotExists,
		})

	}

	return c.Session.ChannelMessageSend(c.ChannelID, message)
}

// ReplyEmbed is a wrapper for sending an embed message to the channel where the message was sent.
// Setting `doReply` to true will make the message a reply to the original message.
func (c *ChannelMessageContext) ReplyEmbed(embed *discordgo.MessageEmbed, doReply ...bool) (*discordgo.Message, error) {
	failIfNotExists := false

	reply := false
	if len(doReply) > 0 {
		reply = doReply[0]
	}

	if reply {
		return c.Session.ChannelMessageSendEmbedReply(c.ChannelID, embed, &discordgo.MessageReference{
			MessageID:       c.Message.ID,
			ChannelID:       c.ChannelID,
			GuildID:         c.Message.GuildID,
			FailIfNotExists: &failIfNotExists,
		})
	}

	return c.Session.ChannelMessageSendEmbed(c.ChannelID, embed)
}
