package minidis

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/tbdsux/minidis/discord/client"
)

// EventContext is passed to the handlers of gateway events.
//
// It is also a context.Context for the handler's own work, canceled when the
// handler returns or a closing bot stops waiting for its handlers. Pass it to
// the calls the handler makes, such as those on Session.
type EventContext struct {
	ctx context.Context
	run context.Context // the bot's run, which outlives the handler

	Bot     *Bot
	Session *client.Session

	// Event is the name of the gateway event, such as "GUILD_MEMBER_ADD".
	Event string
}

func (c *EventContext) context() context.Context {
	if c.ctx == nil {
		return context.Background() // an EventContext built by hand, in a test
	}
	return c.ctx
}

// background returns the context for requests that may be made after the
// handler has returned.
func (c *EventContext) background() context.Context {
	if c.run == nil {
		return context.Background()
	}
	return c.run
}

// Deadline implements context.Context.
func (c *EventContext) Deadline() (time.Time, bool) { return c.context().Deadline() }

// Done implements context.Context.
func (c *EventContext) Done() <-chan struct{} { return c.context().Done() }

// Err implements context.Context.
func (c *EventContext) Err() error { return c.context().Err() }

// Value implements context.Context.
func (c *EventContext) Value(key any) any { return c.context().Value(key) }

// On adds a handler for a gateway event, chosen by the type of the handler's
// second parameter: any of the event types of the discord package, such as
// discord.GuildMemberAdd, discord.MessageReactionAdd or discord.ChannelCreate.
//
//	minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
//		_, err := c.Session.ChannelMessageSend(c, welcomeChannel, "Welcome, "+e.Mention()+"!")
//		return err
//	})
//
// The bot asks Discord for the intents the event needs. Some of them are
// privileged and must also be enabled for the bot in the developer portal:
// those of the member events, of discord.PresenceUpdateEvent, and
// discord.IntentsMessageContent, which a handler of message events that reads
// their content has to ask for with AddIntents. For new messages, OnMessage
// does that itself.
//
// Each event runs its handlers in goroutines of their own. An error or a
// panic goes to the handler set with HandleEventError. On panics if E is not
// an event type.
func On[E any](b *Bot, h func(c *EventContext, e *E) error) {
	name := client.EventName[E]()

	client.Handle(b.Session, func(s *client.Session, e *E) {
		c, done := b.newEvent(s, name)
		defer done()

		b.runEvent(c, func() error { return h(c, e) })
	})

	b.need(client.EventIntents(name))
}

// newEvent makes the context of one run of a handler.
func (b *Bot) newEvent(s *client.Session, name string) (*EventContext, context.CancelFunc) {
	run := b.Minidis.Context()
	ctx, cancel := context.WithCancel(run)

	return &EventContext{ctx: ctx, run: run, Bot: b, Session: s, Event: name}, cancel
}

// runEvent runs a handler and reports its error or panic.
func (b *Bot) runEvent(c *EventContext, h func() error) {
	defer func() {
		if r := recover(); r != nil {
			b.eventError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	if err := h(); err != nil {
		b.eventError(c, err)
	}
}

// HandleEventError sets the function that receives the errors returned by
// event handlers, replacing the default of logging them. A panic arrives as a
// *PanicError. It also receives the errors of message handlers and text
// commands unless HandleMessageError is set. A panic in it is logged.
func (b *Bot) HandleEventError(h func(c *EventContext, err error)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.onEventError = h
}

func (b *Bot) eventError(c *EventContext, err error) {
	b.mu.RLock()
	h := b.onEventError
	b.mu.RUnlock()

	if h == nil {
		b.logError(c.Event+" handler", err)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			b.logError("error handler of "+c.Event, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()
	h(c, err)
}

func (b *Bot) logger() *slog.Logger { return cmp.Or(b.Session.Logger, slog.Default()) }

func (b *Bot) logError(source string, err error) {
	var p *PanicError
	if errors.As(err, &p) {
		b.logger().Error("minidis: "+source, "err", err, "stack", string(p.Stack))
		return
	}
	b.logger().Error("minidis: "+source, "err", err)
}
