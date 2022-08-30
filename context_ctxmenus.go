package minidis

import "github.com/bwmarrin/discordgo"

type MessageCommandContext struct {
	Interaction *discordgo.Interaction
	Session     *discordgo.Session
	Message     *discordgo.Message
}

type UserCommandContext struct {
	Interaction *discordgo.Interaction
	Session     *discordgo.Session
	Member      *discordgo.Member
	User        *discordgo.User
}

func (m *MessageCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.Interaction, reply)
}

func (m *UserCommandContext) ReplyC(reply ReplyProps) error {
	return replyFunc(m.Session, m.Interaction, reply)
}
