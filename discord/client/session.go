// Package client is the REST and Gateway client for the Discord API
// (https://docs.discord.com/developers/intro). The API types it uses are in the
// parent discord package.
//
// A Session is made with New. Its methods call the REST API, each taking a
// context first, and its On methods register handlers for the gateway events
// received once Open has connected:
//
//	s := client.New("Bot " + token)
//	s.OnMessageCreate(func(s *client.Session, e *discord.MessageCreate) {
//		if e.Content == "!ping" {
//			s.ChannelMessageSend(context.Background(), e.ChannelID, "pong")
//		}
//	})
//	if err := s.Open(ctx); err != nil {
//		log.Fatal(err)
//	}
//	defer s.Close()
//
// Open connects every shard of the bot, as many as Discord recommends unless
// Session.TotalShards says otherwise, and their events all go to the same
// handlers. Gateway commands about a guild, such as UpdateVoiceState, go to
// the shard of that guild. To split the shards across processes, give each
// process's Session the shards it runs in Session.Shards.
//
// Endpoints without a typed method can be reached with Session.Request, and
// gateway events without a typed handler with Session.OnRawEvent. Voice is
// limited to joining and leaving channels with UpdateVoiceState: the session
// does not send or receive audio.
//
// Requests follow Discord's rate limits: per-route buckets, the global limit
// of 50 requests a second (kept ahead of time), and 429 responses, including
// Cloudflare's. Requests sharing a bucket are sent one at a time. Once Discord
// rejects the bot token with 401, further requests fail at once rather than
// add to Discord's count of invalid requests. Path parameters are escaped, so
// an ID taken from user input cannot change the path. Use WithAuditLogReason
// to record a reason in a guild's audit log.
package client

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/discord"
)

const (
	// APIVersion is the Discord API and Gateway version in use.
	APIVersion = 10

	// LibraryVersion is reported in the User-Agent header.
	LibraryVersion = "0.3.0"

	libraryName = "minidis"
	libraryURL  = "https://github.com/tbdsux/minidis"
)

// Session is a connection to Discord: a REST client plus a Gateway connection
// for each shard of the bot.
//
// Configure its exported fields before calling Open. Event handlers each run
// in their own goroutine, so a slow handler does not block the gateway, and
// they may run concurrently. A handler that panics is logged and does not stop
// the bot.
type Session struct {
	Token   string // the full Authorization value, including the "Bot " prefix
	Intents discord.Intent

	// TotalShards is how many shards the bot's guilds are split into, each
	// with a gateway connection of its own. With 0, Open uses the number
	// Discord recommends, which is 1 below 2,500 guilds.
	TotalShards int

	// Shards are the shards this Session connects, each from 0 to
	// TotalShards-1. Leave it nil to connect them all; set it, together
	// with TotalShards, to split a bot's shards across processes.
	Shards []int

	// State, if set, caches what the gateway sends; see NewState. The
	// session identifies with its intents as well as Intents.
	State *State

	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client

	// Logger receives what the session logs, such as reconnects, invalid
	// events and panics in handlers. With nil, it uses slog.Default().
	Logger *slog.Logger

	limiter  *rateLimiter
	rejected atomic.Pointer[rejection] // set once Discord rejects the token
	invalid  invalidRequests

	handlersMu  sync.RWMutex
	events      map[string]*eventHandlers // by event name, such as "MESSAGE_CREATE"
	onRawEvents []func(*Session, string, json.RawMessage)

	user  atomic.Pointer[discord.User]
	appID atomic.Pointer[string]

	presence atomic.Pointer[discord.PresenceUpdate] // sent by each shard; see shard.presenceMu

	// lifecycle serializes Open and Shutdown. It is a one-slot channel rather
	// than a mutex so that waiting for it obeys ctx.
	lifecycle chan struct{}
	run       atomic.Pointer[gatewayRun] // the last Open that succeeded; nil before the first

	// active is the run of the last Open that got as far as connecting,
	// whose shards the gateway commands and presence updates go to.
	active atomic.Pointer[gatewayRun]

	// drain holds the handlers of every run whose handlers have not been
	// waited for by a Shutdown yet. Guarded by lifecycle.
	drain []*sync.WaitGroup

	// openingMu guards the cancel func of an Open in progress, so that
	// Shutdown can cancel it without waiting for the lifecycle lock, and the
	// Shutdown counters, which keep an Open that races a Shutdown from
	// reopening the session after it.
	openingMu        sync.Mutex
	opening          context.CancelFunc
	pendingShutdowns int
	shutdowns        uint64 // Shutdowns started so far

	// running tracks the handlers of the current run. It is replaced by Open,
	// before the gateway goroutine that uses it starts.
	running *sync.WaitGroup
}

func (s *Session) logger() *slog.Logger { return cmp.Or(s.Logger, slog.Default()) }

// New creates a Session. token is the full Authorization value, such as "Bot abc123".
func New(token string) *Session {
	return &Session{
		Token:      token,
		Intents:    discord.IntentsAllWithoutPrivileged,
		BaseURL:    fmt.Sprintf("https://discord.com/api/v%d", APIVersion),
		UserAgent:  fmt.Sprintf("DiscordBot (%s, %s)", libraryURL, LibraryVersion),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    newRateLimiter(),
		events:     map[string]*eventHandlers{},
		running:    &sync.WaitGroup{},
		lifecycle:  make(chan struct{}, 1),
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
