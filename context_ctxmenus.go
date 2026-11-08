package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

type MessageCommandContext struct {
	Interaction *discord.Interaction
	Session     *client.Session
	Message     *discord.Message
}

type UserCommandContext struct {
	Interaction *discord.Interaction
	Session     *client.Session
	Member      *discord.Member
	User        *discord.User
}

func (m *MessageCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.Interaction, reply)
}

func (m *UserCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.Interaction, reply)
}
