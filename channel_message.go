package minidis

import (
	"runtime/debug"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// INFO: Too much abstraction? Maybe, will see if this is useful in the future.
type ChannelMessageContext struct {
	ChannelID string
	MessageID string
	Session   *client.Session
	Message   *discord.MessageCreate
}

func (m *Minidis) handleOnMessageCreate(s *client.Session, i *discord.MessageCreate) {
	// Check if we have the handler set for the channel
	if handler, ok := m.ChannelMessageHandlers[i.ChannelID]; ok {
		// If the message is from bot, ignore it
		if i.Author.ID == s.User().ID {
			return
		}

		ctx := &ChannelMessageContext{
			ChannelID: i.ChannelID,
			MessageID: i.ID,
			Session:   s,
			Message:   i,
		}

		m.runChannelHandler(ctx, handler)
		return
	}

	if m.MessageCreateHandler != nil {
		m.MessageCreateHandler(s, i)
	}
}

func (m *Minidis) runChannelHandler(c *ChannelMessageContext, handler func(*ChannelMessageContext) error) {
	defer func() {
		if r := recover(); r != nil {
			m.handleChannelError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	if err := handler(c); err != nil {
		m.handleChannelError(c, err)
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
func (c *ChannelMessageContext) Reply(message string, doReply ...bool) (*discord.Message, error) {
	failIfNotExists := false

	reply := false
	if len(doReply) > 0 {
		reply = doReply[0]
	}

	if reply {
		return c.Session.ChannelMessageSendReply(c.ChannelID, message, &discord.MessageReference{
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
func (c *ChannelMessageContext) ReplyEmbed(embed *discord.MessageEmbed, doReply ...bool) (*discord.Message, error) {
	failIfNotExists := false

	reply := false
	if len(doReply) > 0 {
		reply = doReply[0]
	}

	if reply {
		return c.Session.ChannelMessageSendEmbedReply(c.ChannelID, embed, &discord.MessageReference{
			MessageID:       c.Message.ID,
			ChannelID:       c.ChannelID,
			GuildID:         c.Message.GuildID,
			FailIfNotExists: &failIfNotExists,
		})
	}

	return c.Session.ChannelMessageSendEmbed(c.ChannelID, embed)
}
