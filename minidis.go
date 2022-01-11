package minidis

import "github.com/bwmarrin/discordgo"

type Minidis struct {
	session  *discordgo.Session
	commands []SlashCommandProps
	Token    string
}

func New(token string) *Minidis {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		// FIXME: do not use panic
		panic(err)
	}

	return &Minidis{
		session: s,
		Token:   token,
	}
}
