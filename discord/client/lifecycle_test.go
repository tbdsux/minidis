package client

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// within fails the test unless f returns within d, and returns f's error.
func within(t *testing.T, d time.Duration, what string, f func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- f() }()

	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("%s did not return within %s", what, d)
		return nil
	}
}

func TestRateLimitWaitsStopWhenTheContextEnds(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"retry_after": 30, "global": false}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	err := within(t, 2*time.Second, "a request waiting on retry_after", func() error {
		_, err := s.ChannelMessage(ctx, "1", "2")
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestQueuedRequestsStopWhenTheContextEnds(t *testing.T) {
	release := make(chan struct{})
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		<-release // holds the bucket of the first request
		w.WriteHeader(http.StatusNoContent)
	})
	defer close(release)

	go func() { _ = s.ChannelMessageDelete(context.Background(), "1", "2") }()
	time.Sleep(50 * time.Millisecond) // let it take the bucket

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	err := within(t, 2*time.Second, "a request queued behind its bucket", func() error {
		return s.ChannelMessageDelete(ctx, "1", "3")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// silentGateway accepts the Identify but never sends READY.
func silentGateway(c *fakeConn) {
	c.hello(60000)
	c.read(opIdentify)
	c.waitClosed()
}

func TestOpenStopsWhenTheContextEnds(t *testing.T) {
	s, _ := newFakeGateway(t, silentGateway, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.waitClosed()
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	err := within(t, 2*time.Second, "Open", func() error { return s.Open(ctx) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Open = %v, want context.Canceled", err)
	}
	if s.Done() != nil {
		t.Error("Done is set after a failed Open")
	}

	// The session is reusable.
	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("second Open = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownCancelsAnOpenInProgress(t *testing.T) {
	s, _ := newFakeGateway(t, silentGateway)

	opened := make(chan error, 1)
	go func() { opened <- s.Open(t.Context()) }()
	time.Sleep(100 * time.Millisecond) // Open is waiting for READY

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := within(t, 2*time.Second, "Shutdown", func() error { return s.Shutdown(ctx) }); err != nil {
		t.Errorf("Shutdown = %v", err)
	}

	select {
	case err := <-opened:
		if err == nil {
			t.Error("Open succeeded although Shutdown canceled it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Open did not return after Shutdown")
	}
}

func TestFatalCloseAfterReadyEndsTheSession(t *testing.T) {
	var readies atomic.Int32
	checked := make(chan struct{}) // the test has looked at Err while running
	s, _ := newFakeGateway(t,
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)
			<-checked
			c.closeWith(4014)
			c.waitClosed()
		},
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)
			c.waitClosed()
		},
	)
	s.OnReady(func(*Session, *discord.Ready) { readies.Add(1) })

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Err(); err != nil {
		t.Errorf("Err while running = %v", err)
	}
	close(checked)

	waitFor(t, s.Done(), "the gateway to stop")

	var closeErr *GatewayCloseError
	if err := s.Err(); !errors.As(err, &closeErr) || closeErr.Code != 4014 {
		t.Fatalf("Err = %v, want close code 4014", err)
	}

	// No Close is needed before opening again.
	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open after the fatal close = %v", err)
	}
	if err := s.Err(); err != nil {
		t.Errorf("Err after reopening = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Done stays the closed channel of the last run, so that a caller that
	// reads it after a concurrent Close does not block forever.
	select {
	case <-s.Done():
	default:
		t.Error("Done is not closed after Close")
	}
	if s.Err() != nil {
		t.Errorf("Err after Close = %v", s.Err())
	}
	if n := readies.Load(); n != 2 {
		t.Errorf("READY handlers ran %d times, want 2", n)
	}
}

func TestShutdownWithNoTimeLeftStillStops(t *testing.T) {
	// A select between a free lock and a done context picks at random, so try
	// it often enough to catch the bad pick.
	for range 20 {
		s, _ := newFakeGateway(t, func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)
			c.waitClosed()
		})
		if err := s.Open(t.Context()); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = s.Shutdown(ctx)

		select {
		case <-s.Done():
		default:
			t.Fatal("the gateway is still connected after Shutdown")
		}
	}
}
