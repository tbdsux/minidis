package minidis

import (
	"errors"
	"fmt"
	"log"
	"slices"
)

// ErrNoHandler means an interaction arrived that nothing is registered to
// handle, usually a command Discord still has from before a rename or removal.
// It reaches the error handler wrapped with the command or custom ID.
var ErrNoHandler = errors.New("minidis: no handler registered")

// PanicError is passed to the error handlers when a handler panics.
type PanicError struct {
	Value any
	Stack []byte // the stack of the panic
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("minidis: handler panicked: %v", e.Value)
}

// HandlerFunc is an interaction handler. Component, modal and context menu
// handlers are adapted to it so that middleware can wrap them all.
type HandlerFunc func(c *SlashContext) error

// Middleware wraps an interaction handler. It may run code around next, or
// return without calling it to stop the interaction. c.Interaction.Type says
// which kind of interaction is being handled.
type Middleware func(next HandlerFunc) HandlerFunc

// Use adds middleware to commands, components and modal submits. The first one
// added runs first. Autocomplete is excluded because it fires on every
// keystroke, and channel messages because they are not interactions.
func (m *Minidis) Use(middleware ...Middleware) {
	m.middleware = append(m.middleware, middleware...)
}

// chain wraps handler in the middleware.
func (m *Minidis) chain(handler HandlerFunc) HandlerFunc {
	for _, mw := range slices.Backward(m.middleware) {
		handler = mw(handler)
	}
	return handler
}

// OnError sets the function that receives errors returned by interaction
// handlers and middleware, replacing the default of logging them. A panic
// arrives as a *PanicError. Use it to tell the user something failed, but note
// that the interaction may already have been responded to.
func (m *Minidis) OnError(handler func(c *SlashContext, err error)) {
	m.ErrorHandler = handler
}

// OnChannelMessageError sets the function that receives errors from
// OnChannelMessageCreate handlers, replacing the default of logging them. A
// panic arrives as a *PanicError.
func (m *Minidis) OnChannelMessageError(handler func(c *ChannelMessageContext, err error)) {
	m.ChannelMessageErrorHandler = handler
}

func (m *Minidis) handleError(c *SlashContext, err error) {
	if m.ErrorHandler != nil {
		m.ErrorHandler(c, err)
		return
	}
	logError("interaction "+c.Id, err)
}

func (m *Minidis) handleChannelError(c *ChannelMessageContext, err error) {
	if m.ChannelMessageErrorHandler != nil {
		m.ChannelMessageErrorHandler(c, err)
		return
	}
	logError("channel message handler for "+c.ChannelID, err)
}

func logError(source string, err error) {
	var p *PanicError
	if errors.As(err, &p) {
		log.Printf("minidis: %s: %v\n%s", source, err, p.Stack)
		return
	}
	log.Printf("minidis: %s: %v", source, err)
}
