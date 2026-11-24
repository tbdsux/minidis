package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// gatewayInfoTTL is how long the shards of a run reuse a fetch of GET
// /gateway/bot before identifying again, rather than each fetching it. A
// variable so tests can change it.
var gatewayInfoTTL = 10 * time.Second

// shard is one gateway connection of a run, and the gateway session it keeps
// across reconnects.
type shard struct {
	id    int
	count int // the bot's number of shards

	conn     atomic.Pointer[gatewayConn] // nil until READY or RESUMED
	sequence atomic.Int64                // shared with the heartbeat goroutine; 0 means none yet

	// presenceMu serializes sending the presence on this shard, so that
	// Discord does not end up with an older presence than the stored one.
	// A slow connection then holds up only its own shard.
	presenceMu sync.Mutex

	// Owned by the shard's goroutine while it runs, and by Open before.
	gatewayURL     string
	sessionID      string
	resumeURL      string
	maxConcurrency int  // from GET /gateway/bot
	identified     bool // an Identify was sent since Open
}

func (sh *shard) resetSession() {
	sh.sessionID, sh.resumeURL = "", ""
	sh.sequence.Store(0)
}

// name is how the logs refer to the shard's connection.
func (sh *shard) name() string {
	if sh.count > 1 {
		return fmt.Sprintf("gateway shard %d", sh.id)
	}
	return "gateway"
}

// shardSet is the shards a run connects at a time. It does not change, but
// a run replaces it when Discord asks for more shards.
type shardSet struct {
	count  int      // the bot's number of shards
	shards []*shard // those connected, by ID
}

// newShardSet prepares the shards with the given IDs, out of count, from
// what Discord said in info.
func newShardSet(info *discord.GatewayBot, count int, ids []int) *shardSet {
	ids = slices.Sorted(slices.Values(ids))

	set := &shardSet{count: count}
	for _, id := range ids {
		set.shards = append(set.shards, &shard{
			id:             id,
			count:          count,
			gatewayURL:     info.URL,
			maxConcurrency: info.SessionStartLimit.MaxConcurrency,
		})
	}
	return set
}

// shard returns the shard of set with the given ID, or nil if set does not
// connect it.
func (set *shardSet) shard(id int) *shard {
	i, ok := slices.BinarySearchFunc(set.shards, id, func(sh *shard, id int) int { return sh.id - id })
	if !ok {
		return nil
	}
	return set.shards[i]
}

// openTimeout is how long Open waits for every shard of set to be ready: the
// usual timeout, plus the time the shards of the fullest Identify bucket wait
// for their turn, one after another.
func (set *shardSet) openTimeout() time.Duration {
	most := 0
	buckets := map[int]int{}
	for _, sh := range set.shards {
		b := sh.id % max(sh.maxConcurrency, 1)
		buckets[b]++
		most = max(most, buckets[b])
	}
	return gatewayOpenTimeout + time.Duration(most-1)*identifyInterval
}

// checkShards reports a TotalShards or Shards that Open cannot connect.
func (s *Session) checkShards() error {
	switch {
	case s.TotalShards < 0:
		return fmt.Errorf("discord: TotalShards is %d, want 0 or more", s.TotalShards)
	case s.Shards == nil:
		return nil
	case s.TotalShards == 0:
		return errors.New("discord: Shards needs TotalShards, the bot's total number of shards")
	case len(s.Shards) == 0:
		return errors.New("discord: Shards is empty; leave it nil to connect every shard")
	}

	seen := map[int]bool{}
	for _, id := range s.Shards {
		if id < 0 || id >= s.TotalShards {
			return fmt.Errorf("discord: shard %d is out of range for TotalShards %d", id, s.TotalShards)
		}
		if seen[id] {
			return fmt.Errorf("discord: shard %d is in Shards twice", id)
		}
		seen[id] = true
	}
	return nil
}

// shardIDs returns the number of shards to connect with and the IDs of those
// this Session connects, given what Discord recommends in info. checkShards
// has accepted the configuration.
func (s *Session) shardIDs(info *discord.GatewayBot) (count int, ids []int) {
	count = s.TotalShards
	if count == 0 {
		count = max(info.Shards, 1)
	}
	if s.Shards != nil {
		return count, slices.Clone(s.Shards)
	}
	return count, allShards(count)
}

func allShards(count int) []int {
	ids := make([]int, count)
	for id := range ids {
		ids[id] = id
	}
	return ids
}

// checkSessionStarts reports whether Discord lets n shards identify. Discord
// resets the token of bots that exceed the daily Identify limit.
func checkSessionStarts(info *discord.GatewayBot, n int) error {
	switch limit := info.SessionStartLimit; {
	case limit.Remaining <= 0:
		return fmt.Errorf("discord: no session starts remaining, resets in %s",
			time.Duration(limit.ResetAfter)*time.Millisecond)
	case limit.Remaining < n:
		return fmt.Errorf("discord: %d session starts remaining, for %d shards; resets in %s",
			limit.Remaining, n, time.Duration(limit.ResetAfter)*time.Millisecond)
	}
	return nil
}

// gatewayBot returns what GET /gateway/bot says for a shard about to identify
// again. The shards of a run share a fetch for gatewayInfoTTL, so that many
// shards reconnecting at once do not each wait for one; each counts its
// Identify against the shared answer.
func (r *gatewayRun) gatewayBot(ctx context.Context, s *Session) (*discord.GatewayBot, error) {
	r.infoMu.Lock()
	defer r.infoMu.Unlock()

	if r.info == nil || time.Since(r.infoAt) >= gatewayInfoTTL {
		info, err := s.GatewayBot(ctx)
		if err != nil {
			return nil, err
		}
		r.info, r.infoAt = info, time.Now()
	}

	info := *r.info
	r.info.SessionStartLimit.Remaining--
	return &info, nil
}

// isShardingRequired reports whether err is Discord closing a connection that
// would have had too many guilds.
func isShardingRequired(err error) bool {
	var closeErr *GatewayCloseError
	return errors.As(err, &closeErr) && closeErr.Code == 4011
}

// reshard prepares a new set of shards for a run whose shard count Discord
// refused with close code 4011, once its shards have stopped: as many as
// Discord now recommends, and at least twice as many as before. It waits for
// session starts if Discord has too few left, and retries a failed fetch
// until ctx ends, but not a rejected token.
func (s *Session) reshard(ctx context.Context, run *gatewayRun) (*shardSet, error) {
	old := run.set.Load()

	delay := minReconnectDelay
	for {
		info, err := s.GatewayBot(ctx)
		if err != nil {
			var restErr *RESTError
			if errors.As(err, &restErr) && restErr.StatusCode == http.StatusUnauthorized {
				return nil, err
			}
			log.Printf("discord: resharding: %v", err)
			if err := sleep(ctx, delay); err != nil {
				return nil, err
			}
			delay = min(delay*2, maxReconnectDelay)
			continue
		}

		count := info.Shards
		if count <= old.count {
			count = old.count * 2
		}

		if err := checkSessionStarts(info, count); err != nil {
			wait := time.Duration(info.SessionStartLimit.ResetAfter) * time.Millisecond
			log.Printf("discord: resharding: %v; waiting", err)
			if err := sleep(ctx, max(wait, delay)); err != nil {
				return nil, err
			}
			continue
		}

		// The shards that identify next must not take an old count of
		// session starts for granted.
		run.infoMu.Lock()
		run.info = nil
		run.infoMu.Unlock()

		log.Printf("discord: Discord asked for more shards, reconnecting with %d instead of %d", count, old.count)
		return newShardSet(info, count, allShards(count)), nil
	}
}

// ShardOf returns the shard that receives the events of a guild, and that
// the gateway commands about it go to. It counts the shards that the
// session connected with last, or TotalShards before it has connected. A
// guild ID that is not a number is on shard 0.
func (s *Session) ShardOf(guildID string) int {
	count := s.TotalShards
	if r := s.active.Load(); r != nil {
		count = r.set.Load().count
	}
	return shardOf(guildID, count)
}

// guildShard returns the shard of a guild for a gateway command, or an error
// if the shard cannot be worked out from guildID. With a single shard there
// is nothing to work out, and Discord judges the ID.
func guildShard(guildID string, count int) (int, error) {
	if count <= 1 {
		return 0, nil
	}
	if _, err := strconv.ParseUint(guildID, 10, 64); err != nil {
		return 0, fmt.Errorf("discord: guild ID %q is not a snowflake, so its shard is unknown", guildID)
	}
	return shardOf(guildID, count), nil
}
