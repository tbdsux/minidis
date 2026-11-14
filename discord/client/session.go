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

	handlersMu        sync.RWMutex
	onReady           []func(*Session, *discord.Ready)
	onMessage         []func(*Session, *discord.MessageCreate)
	onMessageUpdate   []func(*Session, *discord.MessageUpdate)
	onMessageDelete   []func(*Session, *discord.MessageDelete)
	onReactionAdd     []func(*Session, *discord.MessageReactionAdd)
	onGuildCreate     []func(*Session, *discord.GuildCreate)
	onGuildMemberAdd  []func(*Session, *discord.GuildMemberAdd)
	onGuildMemberGone []func(*Session, *discord.GuildMemberRemove)
	onInteract        []func(*Session, *discord.InteractionCreate)
	onRawEvents       []func(*Session, string, json.RawMessage)

	user  atomic.Pointer[discord.User]
	appID atomic.Pointer[string]

	conn     atomic.Pointer[gatewayConn] // nil until READY or RESUMED
	presence atomic.Pointer[discord.PresenceUpdate]

	lifecycle sync.Mutex                 // serializes Open, Close and Shutdown
	run       atomic.Pointer[gatewayRun] // the last Open; nil before it and after Shutdown

	// drain holds the handlers of every run since the last Shutdown, which
	// waits for them. Guarded by lifecycle.
	drain []*sync.WaitGroup

	// openingMu guards the cancel func of an Open in progress, so that
	// Shutdown can cancel it without waiting for the lifecycle lock, and the
	// count of Shutdowns waiting, which keeps a racing Open from starting.
	openingMu        sync.Mutex
	opening          context.CancelFunc
	pendingShutdowns int

	// running tracks the handlers of the current run. It is replaced by Open,
	// before the gateway goroutine that uses it starts.
	running *sync.WaitGroup

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
		running:    &sync.WaitGroup{},
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

// addHandler appends h to one of the session's handler lists.
func addHandler[H any](s *Session, list *[]H, h H) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	*list = append(*list, h)
}

// Handlers each run in their own goroutine, so a slow handler does not block
// the gateway, and they may run concurrently. A handler that panics is logged
// and does not stop the bot.

// OnReady registers a handler for the READY event.
func (s *Session) OnReady(h func(*Session, *discord.Ready)) {
	addHandler(s, &s.onReady, h)
}

// OnMessageCreate registers a handler for the MESSAGE_CREATE event.
func (s *Session) OnMessageCreate(h func(*Session, *discord.MessageCreate)) {
	addHandler(s, &s.onMessage, h)
}

// OnMessageUpdate registers a handler for the MESSAGE_UPDATE event.
func (s *Session) OnMessageUpdate(h func(*Session, *discord.MessageUpdate)) {
	addHandler(s, &s.onMessageUpdate, h)
}

// OnMessageDelete registers a handler for the MESSAGE_DELETE event.
func (s *Session) OnMessageDelete(h func(*Session, *discord.MessageDelete)) {
	addHandler(s, &s.onMessageDelete, h)
}

// OnMessageReactionAdd registers a handler for the MESSAGE_REACTION_ADD event.
func (s *Session) OnMessageReactionAdd(h func(*Session, *discord.MessageReactionAdd)) {
	addHandler(s, &s.onReactionAdd, h)
}

// OnGuildCreate registers a handler for the GUILD_CREATE event.
func (s *Session) OnGuildCreate(h func(*Session, *discord.GuildCreate)) {
	addHandler(s, &s.onGuildCreate, h)
}

// OnGuildMemberAdd registers a handler for the GUILD_MEMBER_ADD event, which
// needs the privileged IntentsGuildMembers intent.
func (s *Session) OnGuildMemberAdd(h func(*Session, *discord.GuildMemberAdd)) {
	addHandler(s, &s.onGuildMemberAdd, h)
}

// OnGuildMemberRemove registers a handler for the GUILD_MEMBER_REMOVE event,
// which needs the privileged IntentsGuildMembers intent.
func (s *Session) OnGuildMemberRemove(h func(*Session, *discord.GuildMemberRemove)) {
	addHandler(s, &s.onGuildMemberGone, h)
}

// OnInteractionCreate registers a handler for the INTERACTION_CREATE event.
func (s *Session) OnInteractionCreate(h func(*Session, *discord.InteractionCreate)) {
	addHandler(s, &s.onInteract, h)
}

// OnRawEvent registers a handler that receives every dispatch event as its
// name, such as "GUILD_CREATE", and undecoded JSON payload.
func (s *Session) OnRawEvent(h func(s *Session, event string, data json.RawMessage)) {
	addHandler(s, &s.onRawEvents, h)
}
