package minidis

import "github.com/bwmarrin/discordgo"

type MessageCommandContext struct {
	event   *discordgo.Interaction
	Session *discordgo.Session
	Message *discordgo.Message
}

type UserCommandContext struct {
	event   *discordgo.Interaction
	Session *discordgo.Session
	Member  *discordgo.Member
	User    *discordgo.User
}

func (m *MessageCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.event, reply)
}

func (m *UserCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.event, reply)
}
