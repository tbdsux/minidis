package minidis

import (
	"errors"
	"fmt"
	"log"
	"runtime/debug"
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

// Unwrap returns the panic value if it is an error, so that errors.Is and
// errors.As see through the panic.
func (e *PanicError) Unwrap() error {
	err, _ := e.Value.(error)
	return err
}

// HandlerFunc is an interaction handler. Component, modal and context menu
// handlers are adapted to it so that middleware can wrap them all.
type HandlerFunc func(c *Context) error

// Middleware wraps an interaction handler. It may run code around next, or
// return without calling it to stop the interaction. c.Interaction.Type says
// which kind of interaction is being handled.
type Middleware func(next HandlerFunc) HandlerFunc

// Use adds middleware to commands, components and modal submits. The first one
// added runs first. Autocomplete is excluded because it fires on every
// keystroke, and channel messages because they are not interactions.
func (m *Minidis) Use(middleware ...Middleware) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.middleware = append(m.middleware, middleware...)
}

// chain wraps handler in the middleware.
func (m *Minidis) chain(handler HandlerFunc) HandlerFunc {
	m.mu.RLock()
	middleware := m.middleware
	m.mu.RUnlock()

	for _, mw := range slices.Backward(middleware) {
		handler = mw(handler)
	}
	return handler
}

// HandleError sets the function that receives errors returned by interaction
// handlers and middleware, replacing the default of logging them. A panic
// arrives as a *PanicError. Use it to tell the user something failed, with
// c.Responded deciding between c.Reply and c.Followup. A panic in it is logged.
func (m *Minidis) HandleError(h func(c *Context, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onError = h
}

// HandleChannelMessageError sets the function that receives errors from
// HandleChannelMessage handlers, replacing the default of logging them. A
// panic arrives as a *PanicError. A panic in it is logged.
func (m *Minidis) HandleChannelMessageError(h func(c *ChannelMessageContext, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onChannelError = h
}

func (m *Minidis) handleError(c *Context, err error) {
	m.mu.RLock()
	h := m.onError
	m.mu.RUnlock()

	source := "interaction " + c.ID
	if h == nil {
		logError(source, err)
		return
	}

	defer recoverErrorHandler(source)
	h(c, err)
}

func (m *Minidis) handleChannelError(c *ChannelMessageContext, err error) {
	m.mu.RLock()
	h := m.onChannelError
	m.mu.RUnlock()

	source := "channel message handler for " + c.ChannelID
	if h == nil {
		logError(source, err)
		return
	}

	defer recoverErrorHandler(source)
	h(c, err)
}

// recoverErrorHandler logs a panic in an error handler. Passing it on would
// end the program, as nothing above it recovers on the HTTP path.
func recoverErrorHandler(source string) {
	if r := recover(); r != nil {
		logError("error handler of "+source, &PanicError{Value: r, Stack: debug.Stack()})
	}
}

func logError(source string, err error) {
	var p *PanicError
	if errors.As(err, &p) {
		log.Printf("minidis: %s: %v\n%s", source, err, p.Stack)
		return
	}
	log.Printf("minidis: %s: %v", source, err)
}
