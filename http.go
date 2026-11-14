package minidis

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tbdsux/minidis/discord"
)

const (
	// maxInteractionBody bounds the request body HTTPHandler reads. Real
	// interactions are far smaller.
	maxInteractionBody = 1 << 20

	// signatureMaxAge is how far the signed timestamp may be from now. A
	// replay inside it is caught by the interaction ID instead.
	signatureMaxAge = 5 * time.Minute
)

// HTTPHandler returns an http.Handler for the app's Interactions Endpoint URL,
// so that interactions arrive over HTTP instead of the gateway. publicKey is
// the app's public key, in hex, from the General Information page of the
// Developer Portal. Serve it over HTTPS at the URL set on the same page.
//
// Requests are verified with the public key, replays are rejected, and
// Discord's PING is answered. Other interactions run through the same handlers
// and middleware as gateway interactions, which respond through the API. The
// request is answered once the handler has responded or returned, or after 3
// seconds.
//
// HTTPHandler looks up the bot's user, for Context.Bot, using ctx. Use either
// the gateway or HTTP, not both, as Discord sends interactions to only one.
// Close waits for running handlers, and requests that arrive afterwards get
// 503.
func (m *Minidis) HTTPHandler(ctx context.Context, publicKey string) (http.Handler, error) {
	key, err := hex.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("minidis: the public key must be 64 hex characters")
	}

	if m.Session.User() == nil && m.httpBot.Load() == nil {
		bot := &discord.User{}
		if err := m.Session.Request(ctx, http.MethodGet, "/users/@me", nil, bot); err != nil {
			return nil, fmt.Errorf("minidis: looking up the bot user: %w", err)
		}
		m.httpBot.Store(bot)
	}

	return &httpHandler{m: m, key: ed25519.PublicKey(key), seen: map[string]time.Time{}}, nil
}

type httpHandler struct {
	m   *Minidis
	key ed25519.PublicKey

	mu        sync.Mutex
	seen      map[string]time.Time // interaction IDs received within signatureMaxAge
	lastSweep time.Time
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInteractionBody))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Discord signs the timestamp followed by the body, and sends invalid
	// signatures on purpose to check that they are rejected.
	timestamp := r.Header.Get("X-Signature-Timestamp")
	signature, err := hex.DecodeString(r.Header.Get("X-Signature-Ed25519"))
	if err != nil || timestamp == "" || !ed25519.Verify(h.key, append([]byte(timestamp), body...), signature) {
		http.Error(w, "invalid request signature", http.StatusUnauthorized)
		return
	}

	i := &discord.Interaction{}
	if err := json.Unmarshal(body, i); err != nil {
		http.Error(w, "invalid interaction", http.StatusBadRequest)
		return
	}

	if i.Type == discord.InteractionPing {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(discord.InteractionResponse{Type: discord.InteractionResponsePong})
		return
	}

	// PINGs are exempt, so that the Developer Portal can still verify the URL
	// from a server whose clock is off; they do nothing when replayed.
	now := time.Now()
	if !fresh(timestamp, now) {
		http.Error(w, "stale request signature; check the server's clock", http.StatusUnauthorized)
		return
	}

	if !h.firstSight(i.ID, now) {
		http.Error(w, "interaction already received", http.StatusConflict)
		return
	}

	responded, ok := h.m.startHTTPInteraction(i)
	if !ok {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}

	select {
	case <-responded:
	case <-time.After(responseWindow):
	case <-r.Context().Done():
	}

	// The response went through the API, which Discord asks to acknowledge
	// with 202 and no body.
	w.WriteHeader(http.StatusAccepted)
}

// fresh reports whether the Unix timestamp is within signatureMaxAge of now.
func fresh(timestamp string, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(seconds, 0))
	return age < signatureMaxAge && age > -signatureMaxAge
}

// firstSight records an interaction ID and reports whether it is new. IDs are
// kept as long as their signatures stay fresh, so a replay is caught either
// way. The record is per handler: replicas behind a load balancer each keep
// their own.
func (h *httpHandler) firstSight(id string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.seen[id]; ok {
		return false
	}

	// Forget IDs whose requests would now be rejected as stale anyway. The
	// sweep runs at most once per signatureMaxAge, so the map holds about the
	// last 10 minutes of interactions.
	if now.Sub(h.lastSweep) > signatureMaxAge {
		for seenID, at := range h.seen {
			if now.Sub(at) > 2*signatureMaxAge {
				delete(h.seen, seenID)
			}
		}
		h.lastSweep = now
	}

	h.seen[id] = now
	return true
}

// startHTTPInteraction runs the handler for i in the background. The returned
// channel is closed once it has responded or returned. It reports false if the
// bot has been closed.
func (m *Minidis) startHTTPInteraction(i *discord.Interaction) (<-chan struct{}, bool) {
	m.runMu.RLock()
	run := m.run
	if !run.accepting {
		m.runMu.RUnlock()
		return nil, false
	}
	run.http.Add(1)
	m.runMu.RUnlock()

	responded := make(chan struct{})
	ack := sync.OnceFunc(func() { close(responded) })

	go func() {
		defer run.http.Done()
		defer ack()

		m.runInteraction(run.ctx, m.Session, i, ack)
	}()

	return responded, true
}
