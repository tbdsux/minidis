// Command client is a bot that uses the Discord client on its own, without
// the interactions framework: it answers messages, welcomes new members and
// times out members on request.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func main() {
	s := client.New("Bot " + os.Getenv("TOKEN"))

	// Reading what messages say and seeing members join need privileged
	// intents, which are enabled for the bot in the developer portal.
	s.Intents = discord.IntentsGuilds | discord.IntentsGuildMessages | discord.IntentsMessageContent | discord.IntentsGuildMembers

	s.OnReady(func(s *client.Session, e *discord.Ready) {
		log.Printf("Logged in as %s, in %d guilds", e.User.Username, len(e.Guilds))
	})

	s.OnGuildMemberAdd(func(s *client.Session, e *discord.GuildMemberAdd) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		guild, err := s.Guild(ctx, e.GuildID)
		if err != nil || guild.SystemChannelID == "" {
			return
		}
		if _, err := s.ChannelMessageSend(ctx, guild.SystemChannelID, "Welcome, "+e.Mention()+"!"); err != nil {
			log.Println("welcome:", err)
		}
	})

	s.OnMessageCreate(func(s *client.Session, e *discord.MessageCreate) {
		if e.Author.Bot || e.Member == nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		switch command, _, _ := strings.Cut(e.Content, " "); command {
		case "!ping":
			_, err = s.ChannelMessageSendReply(ctx, e.ChannelID, "pong", &discord.MessageReference{MessageID: e.ID})

		case "!thread":
			_, err = s.MessageThreadStart(ctx, e.ChannelID, e.ID, &discord.ThreadParams{Name: "Thread of " + e.Author.DisplayName()})

		// "!timeout @user" times the mentioned members out for a minute, if
		// the author may do that themselves.
		case "!timeout":
			err = timeout(ctx, s, e)
		}
		if err != nil {
			log.Println(e.Content+":", err)
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := s.Open(ctx); err != nil {
		log.Fatalln("Failed to open the session:", err)
	}
	log.Println("Bot is running. Press Ctrl+C to stop.")

	select {
	case <-ctx.Done():
	case <-s.Done():
		log.Println("The connection ended:", s.Err())
	}

	if err := s.Close(); err != nil {
		log.Println(err)
	}
}

func timeout(ctx context.Context, s *client.Session, e *discord.MessageCreate) error {
	guild, err := s.Guild(ctx, e.GuildID)
	if err != nil {
		return err
	}

	// The member of a message has no User, which the permissions need.
	author := *e.Member
	author.User = e.Author
	if discord.MemberPermissions(guild, nil, &author)&discord.PermissionModerateMembers == 0 {
		_, err := s.ChannelMessageSend(ctx, e.ChannelID, "You are not allowed to time members out.")
		return err
	}

	ctx = client.WithAuditLogReason(ctx, "Timed out by "+e.Author.Username)
	for _, user := range e.Mentions {
		if err := s.GuildMemberTimeout(ctx, e.GuildID, user.ID, time.Now().Add(time.Minute)); err != nil {
			return err
		}
	}
	return s.MessageReactionAdd(ctx, e.ChannelID, e.ID, "✅")
}
