// Package wait lets handlers wait for the next value, such as a click or a
// message, that matches a filter.
package wait

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// ErrPanicked is returned by Wait when its filter panicked.
var ErrPanicked = errors.New("the filter panicked")

// Set holds the waiters for one kind of value. Its zero value is ready to use,
// and it is safe for concurrent use.
type Set[T any] struct {
	mu      sync.Mutex
	waiters []*waiter[T]
}

type waiter[T any] struct {
	ctx    context.Context
	closed <-chan struct{}
	match  func(T) bool
	ch     chan T // buffered; sent to once, or closed if match panicked, by whoever removes the waiter
}

// ended reports whether the waiter has stopped, or is about to, so that no
// value goes to it.
func (w *waiter[T]) ended() bool {
	select {
	case <-w.closed:
		return true
	default:
		return w.ctx.Err() != nil
	}
}

// Wait blocks until Offer hands it a value that match accepts, or match is
// nil, and returns it. It returns ctx's error, or its cause, if ctx ends
// first, and context.Canceled if closed is closed first.
//
// If match panics, the panic goes on to the caller of Offer, and Wait returns
// ErrPanicked: the filter is dropped so that it does not panic for every
// value offered after.
func (s *Set[T]) Wait(ctx context.Context, closed <-chan struct{}, match func(T) bool) (T, error) {
	w := &waiter[T]{ctx: ctx, closed: closed, match: match, ch: make(chan T, 1)}
	s.mu.Lock()
	s.waiters = append(s.waiters, w)
	s.mu.Unlock()

	var err error
	select {
	case v, ok := <-w.ch:
		return received(v, ok)
	case <-ctx.Done():
		err = context.Cause(ctx)
	case <-closed:
		err = context.Canceled
	}

	if !s.remove(w) {
		v, ok := <-w.ch // an Offer took the waiter just as it stopped
		return received(v, ok)
	}
	var zero T
	return zero, err
}

func received[T any](v T, ok bool) (T, error) {
	if !ok {
		return v, ErrPanicked
	}
	return v, nil
}

// Offer gives v to the first waiter that accepts it, in the order they began
// waiting, and returns the context that waiter waits with, which ends once it
// is done with v. It reports false if no waiter took v. Waiters whose context
// has ended, or that are closed, are passed over.
//
// The filters run outside the lock, so that one may wait or offer itself.
func (s *Set[T]) Offer(v T) (context.Context, bool) {
	s.mu.Lock()
	waiters := slices.Clone(s.waiters)
	s.mu.Unlock()

	for _, w := range waiters {
		if !w.ended() && s.accepts(w, v) && s.remove(w) {
			w.ch <- v
			return w.ctx, true
		}
	}
	return nil, false
}

// accepts runs w's filter on v, removing w and waking it if the filter panics.
func (s *Set[T]) accepts(w *waiter[T], v T) bool {
	if w.match == nil {
		return true
	}

	panicked := true
	defer func() {
		if panicked && s.remove(w) {
			close(w.ch)
		}
	}()
	ok := w.match(v)
	panicked = false
	return ok
}

// Len returns the number of waiters.
func (s *Set[T]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.waiters)
}

// remove reports whether w was still waiting, and so is now the caller's.
func (s *Set[T]) remove(w *waiter[T]) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := slices.Index(s.waiters, w)
	if i < 0 {
		return false
	}
	s.waiters = slices.Delete(s.waiters, i, i+1)
	return true
}
