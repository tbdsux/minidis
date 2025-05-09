# minidis

Simple discord bot framework wrapper for [discordgo](https://github.com/bwmarrin/discordgo)

This just wraps functions and is based from the examples provided by [discordgo](https://github.com/bwmarrin/discordgo) and this is made in order to make its api more readable and easier to maintain.

## Install

This is usable for simple and basic commands but is still missing some of other features.

```sh
go get -u github.com/tbdsux/minidis
```

## [CLI](./cli/minidis/README.md)

You can also use a simple boilerplate generator to kickstart a new discord bot project

```sh
go install github.com/tbdsux/minidis/cli/minidis@latest
```

## Usage

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/tbdsux/minidis"
    "github.com/bwmarrin/discordgo"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
    bot := minidis.New(os.Getenv("TOKEN"))

    // Set intents (not required when doing only slash commands)
    bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

    bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
        log.Println("Bot is ready!")
    })

    // simple command
    bot.AddCommand(minidis.SlashCommandProps{
        Command:     "ping",
        Description: "Simple ping command.",
        Execute: func(c *minidis.SlashContext) error {
            return c.ReplyString(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))
        },
    })

    // Open session
	if err := bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Re-sync commands
	if err := bot.ClearCommands(guilds...); err != nil {
		log.Fatalln("Failed to clear commands:", err)
		return
	}
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Run the bot
	bot.Run()

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session
	bot.CloseSession()
}

```

More [examples...](./example/)

##

**&copy; 2022 | tbdsux**
