package interactions

import (
	"cmp"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// Page is one page shown by Paginate: text, embeds or both.
type Page struct {
	Content string
	Embeds  []*discord.MessageEmbed
}

// PaginateOptions configures Paginate. The zero value shows Prev and Next.
type PaginateOptions struct {
	// Timeout is how long the buttons wait for a click before they are
	// disabled; each click starts it again. Zero means 2 minutes.
	Timeout time.Duration

	FirstLast bool // adds buttons to jump to the first and the last page
	Counter   bool // shows the page number, such as "2/5", between the buttons
	Stop      bool // adds a button that ends the paginator at once

	// Anyone lets every user turn the pages, rather than only the one who
	// caused c. Others are told so in an ephemeral message.
	Anyone bool

	// Ephemeral shows the pages only to the user. It has no effect when c
	// was already responded to.
	Ephemeral bool
}

// Paginate responds to c with the first of pages and buttons to turn them,
// then switches pages as they are clicked, until the timeout passes without
// a click, Stop is clicked, c ends or the bot begins to close; it then
// disables the buttons and returns. If c was already responded to, such as
// with DeferReply, the response is edited instead. One page is sent without
// buttons:
//
//	bot.AddCommand(&interactions.Command{
//		Name:        "list",
//		Description: "Lists the items.",
//		Execute: func(c *interactions.Context) error {
//			return bot.Paginate(c, pages, interactions.PaginateOptions{Counter: true})
//		},
//	})
//
// It waits for the clicks with WaitForComponent, so call it from the handler
// of c and return what it returns. As c ends 15 minutes after its
// interaction, so does the paginator.
func (m *Minidis) Paginate(c *Context, pages []Page, opts PaginateOptions) error {
	if len(pages) == 0 {
		return errors.New("interactions: Paginate needs a page")
	}

	p := paginator{pages: pages, opts: opts, prefix: "minidis:page:" + c.ID + ":"}
	first := Message{Content: pages[0].Content, Embeds: pages[0].Embeds, Ephemeral: opts.Ephemeral}
	if len(pages) > 1 {
		first.Components = p.buttons(0, false)
	}
	var err error
	if c.Responded() {
		err = c.EditWith(p.edit(0, len(pages) == 1))
	} else {
		err = c.ReplyWith(first)
	}
	if err != nil || len(pages) == 1 {
		return err
	}

	page, last := 0, c // last is the latest interaction, whose token can still edit the message
	for {
		ctx, cancel := context.WithTimeout(c, cmp.Or(opts.Timeout, 2*time.Minute))
		click, err := m.WaitForComponent(ctx, func(cc *ComponentContext) bool {
			return strings.HasPrefix(cc.Data.CustomID, p.prefix)
		})
		if err != nil {
			cancel()
			// Timed out, c ended or the bot is closing: nothing more can turn the pages.
			return last.EditWith(MessageEdit{Components: p.buttons(page, true)})
		}

		action := strings.TrimPrefix(click.Data.CustomID, p.prefix)
		switch {
		case !opts.Anyone && click.Author.ID != c.Author.ID:
			err = click.ReplyWith(Message{Content: click.translate(KeyPaginateAuthorOnly, "Only <@%s> can turn these pages.", c.Author.ID), Ephemeral: true})
		case action == "stop":
			err = click.UpdateWith(MessageEdit{Components: p.buttons(page, true)})
			cancel()
			return err
		default:
			page = p.turn(page, action)
			err = click.UpdateWith(p.edit(page, false))
			last = click.Context
		}
		cancel()
		if err != nil {
			return errors.Join(err, last.EditWith(MessageEdit{Components: p.buttons(page, true)}))
		}
	}
}

type paginator struct {
	pages  []Page
	opts   PaginateOptions
	prefix string // of the buttons' custom IDs, unique to the paginator
}

// turn returns the page that action leads to from page.
func (p paginator) turn(page int, action string) int {
	switch action {
	case "first":
		return 0
	case "prev":
		return max(page-1, 0)
	case "next":
		return min(page+1, len(p.pages)-1)
	case "last":
		return len(p.pages) - 1
	}
	return page
}

// edit shows page, replacing whatever the message showed before.
func (p paginator) edit(page int, done bool) MessageEdit {
	edit := MessageEdit{
		Content: &p.pages[page].Content,
		Embeds:  append([]*discord.MessageEmbed{}, p.pages[page].Embeds...), // non-nil, so no embeds clears them
	}
	if len(p.pages) > 1 {
		edit.Components = p.buttons(page, done)
	}
	return edit
}

// buttons returns the rows of buttons for page, all disabled once done.
func (p paginator) buttons(page int, done bool) []discord.MessageComponent {
	end := len(p.pages) - 1
	button := func(action, label string, disabled bool) discord.MessageComponent {
		return discord.Button{Label: label, Style: discord.SecondaryButton, CustomID: p.prefix + action, Disabled: done || disabled}
	}

	var row []discord.MessageComponent
	if p.opts.FirstLast {
		row = append(row, button("first", "⏮", page == 0))
	}
	row = append(row, button("prev", "◀", page == 0))
	if p.opts.Counter {
		row = append(row, button("counter", strconv.Itoa(page+1)+"/"+strconv.Itoa(end+1), true))
	}
	row = append(row, button("next", "▶", page == end))
	if p.opts.FirstLast {
		row = append(row, button("last", "⏭", page == end))
	}

	rows := []discord.MessageComponent{discord.ActionsRow{Components: row}}
	if p.opts.Stop {
		stop := discord.Button{Label: "Stop", Style: discord.DangerButton, CustomID: p.prefix + "stop", Disabled: done}
		if len(row) < 5 { // a row holds 5 buttons
			rows[0] = discord.ActionsRow{Components: append(row, stop)}
		} else {
			rows = append(rows, discord.ActionsRow{Components: []discord.MessageComponent{stop}})
		}
	}
	return rows
}
