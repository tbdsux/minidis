package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// TestColdStartBurstHonoursTheSharedBucket sends a burst on a route the
// limiter has never seen. The requests queued behind the first one must move
// to the shared bucket its response names, and wait for it to reset.
func TestColdStartBurstHonoursTheSharedBucket(t *testing.T) {
	const window = 150 * time.Millisecond

	var mu sync.Mutex
	var last time.Time
	var limited atomic.Int32

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		now := time.Now()
		early := !last.IsZero() && now.Sub(last) < window-20*time.Millisecond
		if !early {
			last = now
		}
		mu.Unlock()

		if early {
			limited.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, `{"retry_after": %g, "global": false}`, window.Seconds())
			return
		}

		w.Header().Set("X-RateLimit-Bucket", "shared")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset-After", fmt.Sprintf("%g", window.Seconds()))
		w.WriteHeader(http.StatusNoContent)
	})

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			if err := s.ChannelMessageDelete(t.Context(), "100000000000000001", fmt.Sprint(200000000000000000+i)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if n := limited.Load(); n != 0 {
		t.Errorf("%d requests were sent before the shared bucket reset", n)
	}
}

func TestSweepKeepsBucketsInUse(t *testing.T) {
	l := newRateLimiter()

	held, err := l.acquire(t.Context(), "busy")
	if err != nil {
		t.Fatal(err)
	}

	// Fill the map past the sweep threshold with expired buckets.
	for i := range maxTrackedBuckets + 1 {
		l.bucketFor(fmt.Sprint("filler", i))
	}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := l.acquire(ctx, "busy"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a second request on the route did not wait for the first: %v", err)
	}

	l.release(held, "busy", "", nil)

	b, err := l.acquire(t.Context(), "busy")
	if err != nil {
		t.Fatal(err)
	}
	l.release(b, "busy", "", nil)

	if len(l.buckets) > maxTrackedBuckets {
		t.Errorf("%d buckets after a sweep", len(l.buckets))
	}
}

type panicTransport struct{ panics atomic.Bool }

func (p *panicTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if p.panics.Swap(false) {
		panic("transport bug")
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestPanickingTransportFreesTheRoute(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	transport := &panicTransport{}
	transport.panics.Store(true)
	s.HTTPClient = &http.Client{Transport: transport}

	func() {
		defer func() { _ = recover() }()
		_ = s.ChannelMessageDelete(t.Context(), "1", "2")
	}()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.ChannelMessageDelete(ctx, "1", "3"); err != nil {
		t.Errorf("the route stayed locked after a panic: %v", err)
	}
}

func TestWaitsReturnTheCancelCause(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"retry_after": 30, "global": false}`))
	})

	cause := errors.New("no response in time")
	ctx, cancel := context.WithCancelCause(t.Context())
	time.AfterFunc(50*time.Millisecond, func() { cancel(cause) })

	_, err := s.ChannelMessage(ctx, "1", "2")
	if !errors.Is(err, cause) {
		t.Errorf("err = %v, want the cause", err)
	}
}

func TestOnlyIdempotentRequestsAreRetriedOnServerErrors(t *testing.T) {
	serverRetryDelay = time.Millisecond

	tests := []struct {
		name  string
		call  func(*Session) error
		calls int32
	}{
		{"POST is sent once", func(s *Session) error {
			_, err := s.ChannelMessageSend(t.Context(), "1", "hi")
			return err
		}, 1},
		{"PATCH is sent once", func(s *Session) error {
			_, err := s.ChannelMessageEdit(t.Context(), "1", "2", "hi")
			return err
		}, 1},
		{"GET is retried", func(s *Session) error {
			_, err := s.ChannelMessage(t.Context(), "1", "2")
			return err
		}, 1 + maxServerRetries},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			})

			var restErr *RESTError
			if err := tt.call(s); !errors.As(err, &restErr) || restErr.StatusCode != http.StatusBadGateway {
				t.Errorf("err = %v", err)
			}
			if calls.Load() != tt.calls {
				t.Errorf("calls = %d, want %d", calls.Load(), tt.calls)
			}
		})
	}
}

// TestDrainingShutdownDoesNotBlockOthers has a Shutdown wait for a stuck
// handler. Meanwhile, another Shutdown must give up within its own ctx, and
// an Open must not wait for the drain.
func TestDrainingShutdownDoesNotBlockOthers(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent("stuck"), 2, "MESSAGE_CREATE")
		c.waitClosed()
	}, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.waitClosed()
	})

	var once sync.Once
	s.OnMessageCreate(func(*Session, *discord.MessageCreate) {
		once.Do(func() { close(started) })
		<-release
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, started, "the handler")

	go func() { _ = s.Shutdown(context.Background()) }() // waits for the stuck handler
	time.Sleep(50 * time.Millisecond)

	short, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := within(t, time.Second, "a second Shutdown", func() error { return s.Shutdown(short) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("second Shutdown = %v, want the deadline", err)
	}

	err = within(t, 2*time.Second, "an Open during the drain", func() error { return s.Open(t.Context()) })
	if err != nil {
		t.Errorf("Open during the drain = %v", err)
	}
	_ = s.Shutdown(short)
}

func TestOpenWaitingForTheLockObeysItsContext(t *testing.T) {
	s, _ := newFakeGateway(t, silentGateway)

	go func() { _ = s.Open(context.Background()) }() // holds the lock, waiting for READY
	time.Sleep(50 * time.Millisecond)
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	err := within(t, time.Second, "an Open behind another", func() error { return s.Open(ctx) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Open = %v, want the deadline", err)
	}
}

func TestOpenThatWaitedForAShutdownDoesNotReopen(t *testing.T) {
	s, _ := newFakeGateway(t, silentGateway)

	firstDone := make(chan error, 1)
	go func() { firstDone <- s.Open(context.Background()) }() // holds the lock
	time.Sleep(50 * time.Millisecond)

	second := make(chan error, 1)
	go func() { second <- s.Open(context.Background()) }() // queued on the lock
	time.Sleep(50 * time.Millisecond)

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-firstDone; !errors.Is(err, ErrShuttingDown) {
		t.Errorf("first Open = %v", err)
	}
	select {
	case err := <-second:
		if !errors.Is(err, ErrShuttingDown) {
			t.Errorf("queued Open = %v, want ErrShuttingDown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the queued Open did not return")
	}
}
