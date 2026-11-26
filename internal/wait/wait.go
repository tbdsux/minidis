// Package wait lets handlers wait for the next value, such as a click or a
// message, that matches a filter.
package wait

import (
	"context"
	"slices"
	"sync"
)

// Set holds the waiters for one kind of value. Its zero value is ready to use,
// and it is safe for concurrent use.
type Set[T any] struct {
	mu      sync.Mutex
	waiters []*waiter[T]
}

type waiter[T any] struct {
	ctx   context.Context
	match func(T) bool
	ch    chan T // buffered, and sent to once, by the Offer that removes the waiter
}

// Wait blocks until Offer hands it a value that match accepts, or match is
// nil, and returns it. It returns ctx's error if ctx ends first, and
// context.Canceled if closed is closed first.
func (s *Set[T]) Wait(ctx context.Context, closed <-chan struct{}, match func(T) bool) (T, error) {
	w := &waiter[T]{ctx: ctx, match: match, ch: make(chan T, 1)}
	s.mu.Lock()
	s.waiters = append(s.waiters, w)
	s.mu.Unlock()

	var err error
	select {
	case v := <-w.ch:
		return v, nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-closed:
		err = context.Canceled
	}

	if !s.remove(w) {
		return <-w.ch, nil // an Offer took the waiter just as it stopped
	}
	var zero T
	return zero, err
}

// Offer gives v to the first waiter that accepts it, in the order they began
// waiting, and returns the context that waiter waits with, which ends once it
// is done with v. It reports false if no waiter took v.
//
// The filters run outside the lock, so that one may wait or offer itself.
func (s *Set[T]) Offer(v T) (context.Context, bool) {
	s.mu.Lock()
	waiters := slices.Clone(s.waiters)
	s.mu.Unlock()

	for _, w := range waiters {
		if (w.match == nil || w.match(v)) && s.remove(w) {
			w.ch <- v
			return w.ctx, true
		}
	}
	return nil, false
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
