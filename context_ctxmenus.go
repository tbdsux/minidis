package minidis

import "github.com/bwmarrin/discordgo"

type MessageCommandContext struct {
	event   *discordgo.Interaction
	session *discordgo.Session
	Message *discordgo.Message
}

type UserCommandContext struct {
	event   *discordgo.Interaction
	session *discordgo.Session
	Member  *discordgo.Member
	User    *discordgo.User
}

func (m *MessageCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.session, m.event, reply)
}

func (m *UserCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.session, m.event, reply)
}
