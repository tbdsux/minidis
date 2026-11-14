package minidis

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tbdsux/minidis/discord"
)

const (
	// maxInteractionBody bounds the request body HTTPHandler reads.
	maxInteractionBody = 4 << 20

	// initialResponseWindow is how long Discord waits for the first response.
	initialResponseWindow = 3 * time.Second
)

// HTTPHandler returns an http.Handler for the app's Interactions Endpoint URL,
// so that interactions arrive over HTTP instead of the gateway. publicKey is
// the app's public key, in hex, from the General Information page of the
// Developer Portal. Serve it over HTTPS at the URL set on the same page.
//
// Requests are verified with the public key and Discord's PING is answered.
// Other interactions run through the same handlers and middleware as gateway
// interactions, which respond through the API. The request is answered once
// the handler has responded or returned, or after 3 seconds.
//
// The gateway is not needed, so Open can be skipped; then c.Bot is nil. Close
// waits for running handlers, and requests that arrive afterwards get 503.
func (m *Minidis) HTTPHandler(publicKey string) (http.Handler, error) {
	key, err := hex.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("minidis: the public key must be 64 hex characters")
	}

	return &httpHandler{m: m, key: ed25519.PublicKey(key)}, nil
}

type httpHandler struct {
	m   *Minidis
	key ed25519.PublicKey
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
	// yagni: no replay window; a replayed interaction's token is already used.
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

	responded, ok := h.m.startHTTPInteraction(i)
	if !ok {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}

	select {
	case <-responded:
	case <-time.After(initialResponseWindow):
	case <-r.Context().Done():
	}

	// The response went through the API, which Discord asks to acknowledge
	// with 202 and no body.
	w.WriteHeader(http.StatusAccepted)
}

// startHTTPInteraction runs the handler for i in the background. The returned
// channel is closed once it has responded or returned. It reports false if the
// bot has been closed.
func (m *Minidis) startHTTPInteraction(i *discord.Interaction) (<-chan struct{}, bool) {
	m.runMu.RLock()
	run := m.run
	if run.ctx.Err() != nil {
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
