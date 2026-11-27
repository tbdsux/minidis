package interactions

import (
	"context"
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// ComponentContext is passed to the handler of a button or select menu.
type ComponentContext struct {
	*Context
	Data discord.MessageComponentInteractionData

	// Params holds the captures of a handler ID such as "vote:{id}".
	Params map[string]string
}

// HandleComponent sets the handler for the components with the custom ID id.
//
// The id can capture parts of a custom ID: segments are separated by ":", and
// one written as "{name}" matches anything, so "vote:{poll}" handles "vote:42"
// with c.Params["poll"] set to "42". A capture is one segment, so the value
// put there must not contain ":". An exact id takes priority over a pattern,
// and a pattern with more literal segments over one with fewer.
func (m *Minidis) HandleComponent(id string, h func(c *ComponentContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.components.add(id, h)
}

// HandleComponentFallback sets the handler for components that no
// HandleComponent id matches. Without one, they reach the error handler as
// ErrNoHandler; when other code handles some components, such as a collector
// on Session.OnInteractionCreate (which sees gateway interactions only), set
// one that returns nil.
func (m *Minidis) HandleComponentFallback(h func(c *ComponentContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.componentFallback = h
}

// WaitForComponent waits for the next use of a button or select menu that
// filter accepts, or of any if filter is nil, and returns it to be responded
// to, such as with Update. A command uses it to ask for a confirmation:
//
//	ctx, cancel := context.WithTimeout(c, time.Minute)
//	defer cancel()
//
//	click, err := m.WaitForComponent(ctx, func(cc *ComponentContext) bool {
//		return cc.Author.ID == c.Author.ID && cc.Data.CustomID == "confirm:"+c.ID
//	})
//	if err != nil {
//		return c.Edit("Timed out.")
//	}
//	return click.Update("Done.")
//
// It works for interactions from the gateway and from HTTPHandler alike. A
// component that it takes goes through middleware but not to the handler of
// HandleComponent or HandleComponentFallback, and its Params are empty. The
// first waiter to begin gets it if several accept it. filter may run for
// several components at once, after middleware.
//
// It returns ctx's error if ctx ends first, and context.Canceled once the bot
// closes. If filter panics, the panic goes to the error handler of the
// component it ran for, and WaitForComponent returns an error.
//
// The Context it returns is canceled when ctx ends, which the caller
// should therefore end when it is done; its response must still come within
// 3 seconds of the click.
func (m *Minidis) WaitForComponent(ctx context.Context, filter func(c *ComponentContext) bool) (*ComponentContext, error) {
	return m.componentWaiters.Wait(ctx, m.Context().Done(), filter)
}

func (m *Minidis) routeComponent(c *Context) HandlerFunc {
	data := c.Interaction.MessageComponentData()

	h, params, ok := m.components.find(data.CustomID)
	if !ok {
		h = m.componentFallback
	}

	return func(c *Context) error {
		cc := &ComponentContext{Context: c, Data: data}

		// The waiter responds, with c kept open until it is done.
		if waiting, ok := m.componentWaiters.Offer(cc); ok {
			select {
			case <-waiting.Done():
			case <-c.Done():
			}
			return nil
		}

		if h == nil {
			return fmt.Errorf("%w: component %q", ErrNoHandler, data.CustomID)
		}
		cc.Params = params
		return h(cc)
	}
}

// Update responds by replacing the text of the message the component is on.
func (c *ComponentContext) Update(content string) error {
	return c.update(MessageEdit{Content: &content})
}

// UpdateWith responds by changing the message the component is on.
func (c *ComponentContext) UpdateWith(edit MessageEdit) error {
	return c.update(edit)
}

// DeferUpdate acknowledges the interaction without changing the message yet.
// Change it within 15 minutes using Edit.
func (c *ComponentContext) DeferUpdate() error {
	return c.deferUpdate()
}
