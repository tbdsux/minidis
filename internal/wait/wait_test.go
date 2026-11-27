package wait

import (
	"context"
	"errors"
	"testing"
)

func TestAPanickingFilterIsDropped(t *testing.T) {
	var s Set[int]
	bad := make(chan error, 1)
	good := make(chan int, 1)
	go func() {
		_, err := s.Wait(t.Context(), nil, func(int) bool { panic("boom") })
		bad <- err
	}()
	for s.Len() != 1 {
	}
	go func() {
		v, _ := s.Wait(t.Context(), nil, nil)
		good <- v
	}()
	for s.Len() != 2 {
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic did not reach the caller of Offer")
			}
		}()
		s.Offer(1)
	}()
	if err := <-bad; !errors.Is(err, ErrPanicked) {
		t.Errorf("err = %v, want ErrPanicked", err)
	}
	if _, ok := s.Offer(2); !ok || <-good != 2 {
		t.Error("the next waiter did not get the value")
	}
}

func TestOfferPassesOverEndedWaiters(t *testing.T) {
	var s Set[int]
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	closed := make(chan struct{})
	close(closed)

	// As they are between ending and removing themselves.
	s.waiters = append(s.waiters,
		&waiter[int]{ctx: ctx, ch: make(chan int, 1)},
		&waiter[int]{ctx: t.Context(), closed: closed, ch: make(chan int, 1)})
	if _, ok := s.Offer(1); ok {
		t.Error("an ended waiter took the value")
	}
}

func TestWaitReturnsTheCause(t *testing.T) {
	var s Set[int]
	cause := errors.New("cause")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if _, err := s.Wait(ctx, nil, nil); !errors.Is(err, cause) {
		t.Errorf("err = %v, want the cause", err)
	}
}
