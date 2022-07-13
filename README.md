# minidis

Simple slash commands handler for [discordgo](https://github.com/bwmarrin/discordgo)

This just wraps functions and is based from the examples provided by [discordgo](https://github.com/bwmarrin/discordgo) and this is made in order to make its api more readable and easier to maintain.

## Install

This is usable for simple and basic commands but is still missing some of other features.

```sh
go get -u github.com/TheBoringDude/minidis
```

## [CLI](./cli/minidis/README.md)

You can also use a simple boilerplate generator to kickstart a new discord bot project

```sh
go install github.com/TheBoringDude/minidis/cli/minidis@latest
```

## Usage

```go
package main

import (
    "fmt"
    "log"
    "os"

    "github.com/TheBoringDude/minidis"
    "github.com/bwmarrin/discordgo"
)

func main() {
    bot := minidis.New(os.Getenv("TOKEN"))

    // set intents
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

    bot.Run()
}

```

More [examples...](./example/)

##

**&copy; 2022 | TheBoringDude**
