package client

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxTrackedBuckets is how many buckets are kept before expired ones are dropped.
// Every interaction token gets its own webhook bucket, so the map would otherwise grow forever.
const maxTrackedBuckets = 2048

type bucket struct {
	// gate serializes requests in this bucket and is held across the HTTP call.
	// It is a channel rather than a mutex so that waiting for it obeys ctx.
	gate chan struct{}

	// guarded by rateLimiter.mu
	limited   bool
	remaining int
	reset     time.Time
}

// rateLimiter follows the X-RateLimit-* headers. Routes start in their own
// bucket and move to the shared one once Discord reports its bucket ID.
type rateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	routes      map[string]string
	globalUntil time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]*bucket{}, routes: map[string]string{}}
}

func (l *rateLimiter) bucketFor(route string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) >= maxTrackedBuckets {
		l.sweep()
	}

	key, ok := l.routes[route]
	if !ok {
		key = route
	}

	return l.getOrCreate(key)
}

func (l *rateLimiter) getOrCreate(key string) *bucket {
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{gate: make(chan struct{}, 1)}
		l.buckets[key] = b
	}
	return b
}

func (l *rateLimiter) sweep() {
	now := time.Now()
	for key, b := range l.buckets {
		if b.reset.Before(now) {
			delete(l.buckets, key)
		}
	}
	for route, key := range l.routes {
		if _, ok := l.buckets[key]; !ok {
			delete(l.routes, route)
		}
	}
}

// acquire blocks until a request may be sent for route. The caller must release the bucket.
func (l *rateLimiter) acquire(ctx context.Context, route string) (*bucket, error) {
	b := l.bucketFor(route)
	select {
	case b.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	l.mu.Lock()
	wait := time.Until(l.globalUntil)
	if b.limited && b.remaining <= 0 {
		wait = max(wait, time.Until(b.reset))
	}
	l.mu.Unlock()

	if err := sleep(ctx, wait); err != nil {
		<-b.gate
		return nil, err
	}

	return b, nil
}

// release records the response headers (nil if the request failed) and frees the bucket.
func (l *rateLimiter) release(b *bucket, route, major string, h http.Header) {
	if h != nil {
		l.update(b, route, major, h)
	}
	<-b.gate
}

func (l *rateLimiter) update(b *bucket, route, major string, h http.Header) {
	remaining, errRemaining := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	resetAfter, errReset := strconv.ParseFloat(h.Get("X-RateLimit-Reset-After"), 64)

	l.mu.Lock()
	defer l.mu.Unlock()

	target := b
	if id := h.Get("X-RateLimit-Bucket"); id != "" {
		key := id + "|" + major
		target = l.getOrCreate(key)
		if previous, ok := l.routes[route]; !ok || previous != key {
			l.routes[route] = key
			delete(l.buckets, route)
		}
	}

	if errRemaining == nil && errReset == nil {
		target.limited = true
		target.remaining = remaining
		target.reset = time.Now().Add(seconds(resetAfter))
	}
}

func (l *rateLimiter) setGlobal(wait time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if until := time.Now().Add(wait); until.After(l.globalUntil) {
		l.globalUntil = until
	}
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// sleep waits for d, returning early with the context's error if it is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// routeKey groups a request path by rate limit route. Top-level resources stay
// literal because Discord limits per resource; other snowflakes are collapsed.
// major is that literal part, used to scope a shared bucket ID.
func routeKey(method, path string) (route, major string) {
	path, _, _ = strings.Cut(path, "?")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	var kept []string
	for i, seg := range segments {
		switch {
		case i == 1 && (segments[0] == "channels" || segments[0] == "guilds"),
			(i == 1 || i == 2) && segments[0] == "webhooks":
			kept = append(kept, seg)
		case isSnowflake(seg):
			segments[i] = ":id"
		}
	}

	return method + " /" + strings.Join(segments, "/"), strings.Join(kept, "/")
}

func isSnowflake(s string) bool {
	if len(s) < 15 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// redactPath removes interaction and webhook tokens so paths are safe to log.
func redactPath(path string) string {
	path, _, _ = strings.Cut(path, "?")
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(segments) >= 3 && (segments[0] == "webhooks" || segments[0] == "interactions") {
		segments[2] = ":token"
	}

	return "/" + strings.Join(segments, "/")
}
