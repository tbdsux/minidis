package minidis

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// ChannelMessageContext is passed to OnChannelMessageCreate handlers.
//
// It is also a context.Context, canceled when the handler returns or the bot
// closes.
type ChannelMessageContext struct {
	ctx context.Context

	Session   *client.Session
	Message   *discord.MessageCreate
	ChannelID string
	MessageID string
}

// Deadline implements context.Context.
func (c *ChannelMessageContext) Deadline() (time.Time, bool) { return orBackground(c.ctx).Deadline() }

// Done implements context.Context.
func (c *ChannelMessageContext) Done() <-chan struct{} { return orBackground(c.ctx).Done() }

// Err implements context.Context.
func (c *ChannelMessageContext) Err() error { return orBackground(c.ctx).Err() }

// Value implements context.Context.
func (c *ChannelMessageContext) Value(key any) any { return orBackground(c.ctx).Value(key) }

// OnChannelMessageCreate sets the handler for messages in the given channels.
// The bot's own messages are ignored. Reading message content needs the
// privileged discord.IntentsMessageContent intent.
func (m *Minidis) OnChannelMessageCreate(h func(c *ChannelMessageContext) error, channels ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, channel := range channels {
		m.channelHandlers[channel] = h
	}
}

func (m *Minidis) handleChannelMessage(s *client.Session, msg *discord.MessageCreate) {
	m.mu.RLock()
	h := m.channelHandlers[msg.ChannelID]
	m.mu.RUnlock()

	if h == nil || msg.Author == nil {
		return
	}

	// Replying to its own messages could loop forever.
	if bot := s.User(); bot != nil && msg.Author.ID == bot.ID {
		return
	}

	ctx, cancel := context.WithCancel(m.baseContext())
	defer cancel()

	m.runChannelHandler(&ChannelMessageContext{
		ctx:       ctx,
		Session:   s,
		Message:   msg,
		ChannelID: msg.ChannelID,
		MessageID: msg.ID,
	}, h)
}

func (m *Minidis) runChannelHandler(c *ChannelMessageContext, h func(*ChannelMessageContext) error) {
	defer func() {
		if r := recover(); r != nil {
			m.handleChannelError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	if err := h(c); err != nil {
		m.handleChannelError(c, err)
	}
}

// reference points at the message being handled. It does not fail if the
// message was deleted in the meantime.
func (c *ChannelMessageContext) reference() *discord.MessageReference {
	return &discord.MessageReference{
		MessageID:       c.Message.ID,
		ChannelID:       c.ChannelID,
		GuildID:         c.Message.GuildID,
		FailIfNotExists: discord.Bool(false),
	}
}

// Send sends a message to the channel.
func (c *ChannelMessageContext) Send(content string) (*discord.Message, error) {
	return c.Session.ChannelMessageSend(c.ChannelID, content)
}

// SendEmbed sends an embed to the channel.
func (c *ChannelMessageContext) SendEmbed(embed *discord.MessageEmbed) (*discord.Message, error) {
	return c.Session.ChannelMessageSendEmbed(c.ChannelID, embed)
}

// Reply sends a message to the channel as a reply to the message.
func (c *ChannelMessageContext) Reply(content string) (*discord.Message, error) {
	return c.Session.ChannelMessageSendReply(c.ChannelID, content, c.reference())
}

// ReplyEmbed sends an embed to the channel as a reply to the message.
func (c *ChannelMessageContext) ReplyEmbed(embed *discord.MessageEmbed) (*discord.Message, error) {
	return c.Session.ChannelMessageSendEmbedReply(c.ChannelID, embed, c.reference())
}
