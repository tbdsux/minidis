// Package client is the REST and Gateway client for the Discord API
// (https://docs.discord.com/developers/intro). The API types it uses are in the
// parent discord package.
//
// Endpoints without a typed method can be reached with Session.Request, and
// gateway events without a typed handler with Session.OnRawEvent.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tbdsux/minidis/discord"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// APIVersion is the Discord API and Gateway version in use.
	APIVersion = 10

	// LibraryVersion is reported in the User-Agent header.
	LibraryVersion = "0.1.0"

	libraryName = "minidis"
	libraryURL  = "https://github.com/tbdsux/minidis"
)

// Session is a connection to Discord: a REST client plus a Gateway connection.
//
// Configure its exported fields before calling Open.
type Session struct {
	Token      string // the full Authorization value, including the "Bot " prefix
	Intents    discord.Intent
	ShardID    int
	ShardCount int // sharding is only enabled when this is greater than 1

	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client

	limiter *rateLimiter

	handlersMu  sync.RWMutex
	onReady     []func(*Session, *discord.Ready)
	onMessage   []func(*Session, *discord.MessageCreate)
	onInteract  []func(*Session, *discord.InteractionCreate)
	onRawEvents []func(*Session, string, json.RawMessage)

	user  atomic.Pointer[discord.User]
	appID atomic.Pointer[string]

	lifecycle sync.Mutex // serializes Open and Close
	cancel    context.CancelFunc
	done      chan struct{}

	// Owned by the gateway goroutine while it runs.
	gatewayURL   string
	sessionID    string
	resumeURL    string
	lastIdentify time.Time
	identified   bool

	sequence atomic.Int64 // shared with the heartbeat goroutine; 0 means none yet
}

// New creates a Session. token is the full Authorization value, such as "Bot abc123".
func New(token string) *Session {
	return &Session{
		Token:      token,
		Intents:    discord.IntentsAllWithoutPrivileged,
		BaseURL:    fmt.Sprintf("https://discord.com/api/v%d", APIVersion),
		UserAgent:  fmt.Sprintf("DiscordBot (%s, %s)", libraryURL, LibraryVersion),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    newRateLimiter(),
	}
}

// User returns the bot's user, or nil before the session is ready.
func (s *Session) User() *discord.User {
	return s.user.Load()
}

// ApplicationID returns the bot's application ID, or "" before the session is ready.
func (s *Session) ApplicationID() string {
	if id := s.appID.Load(); id != nil {
		return *id
	}
	return ""
}

// Handlers each run in their own goroutine, so a slow handler does not block
// the gateway, and they may run concurrently.

// OnReady registers a handler for the READY event.
func (s *Session) OnReady(h func(*Session, *discord.Ready)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onReady = append(s.onReady, h)
}

// OnMessageCreate registers a handler for the MESSAGE_CREATE event.
func (s *Session) OnMessageCreate(h func(*Session, *discord.MessageCreate)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onMessage = append(s.onMessage, h)
}

// OnInteractionCreate registers a handler for the INTERACTION_CREATE event.
func (s *Session) OnInteractionCreate(h func(*Session, *discord.InteractionCreate)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onInteract = append(s.onInteract, h)
}

// OnRawEvent registers a handler that receives every dispatch event as its
// name, such as "GUILD_CREATE", and undecoded JSON payload.
func (s *Session) OnRawEvent(h func(s *Session, event string, data json.RawMessage)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onRawEvents = append(s.onRawEvents, h)
}
