package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// Guilds on shard 0 and shard 1 of 2: a guild's shard is (id >> 22) % count.
const (
	guildOnShard0 = "8388608" // 2 << 22
	guildOnShard1 = "4194304" // 1 << 22
)

type sentCommand struct {
	shard int
	op    int
	d     json.RawMessage
}

// shardFake scripts gateway connections that each identify as a shard.
type shardFake struct {
	identified chan *[2]int     // the shard each connection identified as, nil without one
	commands   chan sentCommand // what the connections received after READY, but heartbeats
	ended      chan int         // the shards whose connection has closed

	afterReady func(c *fakeConn, shard, count int) // if set, runs once READY is sent
}

func newShardFake() *shardFake {
	return &shardFake{
		identified: make(chan *[2]int, 16),
		commands:   make(chan sentCommand, 16),
		ended:      make(chan int, 16),
	}
}

// script identifies a shard, sends READY for it, then records the commands
// the connection receives until it closes.
func (f *shardFake) script(t *testing.T) func(c *fakeConn) {
	return func(c *fakeConn) {
		c.hello(60000)

		var identify struct {
			Shard *[2]int `json:"shard"`
		}
		if err := json.Unmarshal(c.read(opIdentify).D, &identify); err != nil {
			t.Error(err)
		}
		f.identified <- identify.Shard

		shard, count := 0, 1
		if identify.Shard != nil {
			shard, count = identify.Shard[0], identify.Shard[1]
		}
		defer func() { f.ended <- shard }()

		c.send(opDispatch, map[string]any{
			"v": 10, "session_id": fmt.Sprintf("sess-%d", shard), "resume_gateway_url": "ws://" + c.host + "/resume",
			"user": map[string]any{"id": "42", "username": "bot"}, "guilds": []any{}, "shard": identify.Shard,
		}, 1, "READY")

		if f.afterReady != nil {
			f.afterReady(c, shard, count)
		}

		_ = c.ws.SetReadDeadline(time.Time{})
		for {
			var p gatewayPayload
			if err := c.ws.ReadJSON(&p); err != nil {
				return
			}
			if p.Op != opHeartbeat {
				f.commands <- sentCommand{shard, p.Op, p.D}
			}
		}
	}
}

// identifiedShards collects the shards that n connections identified as.
func (f *shardFake) identifiedShards(t *testing.T, n int) [][2]int {
	t.Helper()
	var got [][2]int
	for range n {
		select {
		case s := <-f.identified:
			if s == nil {
				t.Fatal("a connection identified without a shard")
			}
			got = append(got, *s)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for the Identify of %d shards", n)
		}
	}
	slices.SortFunc(got, func(a, b [2]int) int { return a[0] - b[0] })
	return got
}

// command waits for the next command a connection received.
func (f *shardFake) command(t *testing.T) sentCommand {
	t.Helper()
	select {
	case c := <-f.commands:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a gateway command")
		return sentCommand{}
	}
}

func repeat[T any](v T, n int) []T {
	return slices.Repeat([]T{v}, n)
}

func TestOpenConnectsTheRecommendedShards(t *testing.T) {
	f := newShardFake()
	f.afterReady = func(c *fakeConn, shard, _ int) {
		c.send(opDispatch, map[string]any{
			"id": fmt.Sprint("m", shard), "channel_id": "c1", "guild_id": "g", "content": "hi", "author": map[string]string{"id": "7"},
		}, 2, "MESSAGE_CREATE")
	}
	s, g := newFakeGateway(t, repeat(f.script(t), 3)...)
	g.shards.Store(3)

	var readies, messages atomic.Int32
	seen := make(chan struct{}, 6)
	s.OnReady(func(_ *Session, r *discord.Ready) {
		if r.Shard == nil || r.Shard[1] != 3 {
			t.Errorf("READY shard = %v", r.Shard)
		}
		readies.Add(1)
		seen <- struct{}{}
	})
	s.OnMessageCreate(func(*Session, *discord.MessageCreate) {
		messages.Add(1)
		seen <- struct{}{}
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	if got, want := f.identifiedShards(t, 3), [][2]int{{0, 3}, {1, 3}, {2, 3}}; !slices.Equal(got, want) {
		t.Errorf("identified shards = %v, want %v", got, want)
	}
	if n := g.conns.Load(); n != 3 {
		t.Errorf("connections = %d, want 3", n)
	}
	if n := g.starts.Load(); n != 1 {
		t.Errorf("GET /gateway/bot = %d times, want 1 for every shard", n)
	}

	for range 6 {
		select {
		case <-seen:
		case <-time.After(5 * time.Second):
			t.Fatalf("handlers ran for %d READY and %d MESSAGE_CREATE, want 3 of each", readies.Load(), messages.Load())
		}
	}

	// 2 << 22 is on shard 2 of 3.
	if got := s.ShardOf(guildOnShard0); got != 2 {
		t.Errorf("ShardOf = %d, want 2 with 3 shards", got)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		select {
		case <-f.ended:
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not close every shard")
		}
	}
}

func TestOpenWithOneShardSendsNoShard(t *testing.T) {
	f := newShardFake()
	s, _ := newFakeGateway(t, f.script(t))

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	select {
	case shard := <-f.identified:
		if shard != nil {
			t.Errorf("Identify has shard %v, want none for an unsharded bot", shard)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the Identify")
	}
}

func TestOpenConnectsTheGivenShards(t *testing.T) {
	f := newShardFake()
	s, g := newFakeGateway(t, repeat(f.script(t), 2)...)
	g.shards.Store(1) // TotalShards wins over the recommendation
	s.TotalShards, s.Shards = 4, []int{3, 1}

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	if got, want := f.identifiedShards(t, 2), [][2]int{{1, 4}, {3, 4}}; !slices.Equal(got, want) {
		t.Errorf("identified shards = %v, want %v", got, want)
	}
	if n := g.conns.Load(); n != 2 {
		t.Errorf("connections = %d, want 2", n)
	}
}

func TestOpenRejectsInvalidShards(t *testing.T) {
	cases := []struct {
		name   string
		count  int
		shards []int
		want   string
	}{
		{"negative count", -1, nil, "TotalShards is -1"},
		{"shards without count", 0, []int{0}, "Shards needs TotalShards"},
		{"empty shards", 2, []int{}, "Shards is empty"},
		{"shard out of range", 2, []int{2}, "out of range"},
		{"negative shard", 2, []int{-1}, "out of range"},
		{"shard twice", 2, []int{1, 1}, "twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, g := newFakeGateway(t)
			s.TotalShards, s.Shards = tc.count, tc.shards

			if err := s.Open(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Open = %v, want an error about %q", err, tc.want)
			}
			if n := g.starts.Load(); n != 0 {
				t.Errorf("GET /gateway/bot = %d times, want none", n)
			}
		})
	}
}

func TestOpenNeedsASessionStartForEachShard(t *testing.T) {
	s, g := newFakeGateway(t)
	g.shards.Store(3)
	g.remain.Store(2)

	if err := s.Open(t.Context()); err == nil || !strings.Contains(err.Error(), "2 session starts remaining, for 3 shards") {
		t.Fatalf("Open = %v", err)
	}
	if n := g.conns.Load(); n != 0 {
		t.Errorf("connections = %d, want 0", n)
	}
}

func TestGatewayCommandsGoToTheShardOfTheirGuild(t *testing.T) {
	f := newShardFake()
	s, g := newFakeGateway(t, repeat(f.script(t), 2)...)
	g.shards.Store(2)

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	guildOf := func(c sentCommand) string {
		var d struct {
			GuildID  string   `json:"guild_id"`
			GuildIDs []string `json:"guild_ids"`
		}
		if err := json.Unmarshal(c.d, &d); err != nil {
			t.Fatal(err)
		}
		if d.GuildIDs != nil {
			return strings.Join(d.GuildIDs, ",")
		}
		return d.GuildID
	}

	if err := s.UpdateVoiceState(guildOnShard1, "c1", false, true); err != nil {
		t.Fatal(err)
	}
	if c := f.command(t); c.shard != 1 || c.op != opVoiceStateUpdate || guildOf(c) != guildOnShard1 {
		t.Errorf("voice state update went to shard %d as op %d for guild %s", c.shard, c.op, guildOf(c))
	}

	if err := s.RequestGuildMembers(discord.GuildMembersRequest{GuildID: guildOnShard0}); err != nil {
		t.Fatal(err)
	}
	if c := f.command(t); c.shard != 0 || c.op != opRequestGuildMembers || guildOf(c) != guildOnShard0 {
		t.Errorf("member request went to shard %d as op %d for guild %s", c.shard, c.op, guildOf(c))
	}

	// One request for the guilds of each shard.
	if err := s.RequestSoundboardSounds(guildOnShard0, guildOnShard1, "0"); err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for range 2 {
		c := f.command(t)
		if c.op != opRequestSoundboardSounds {
			t.Fatalf("got op %d, want a soundboard request", c.op)
		}
		got[c.shard] = guildOf(c)
	}
	if got[0] != guildOnShard0+",0" || got[1] != guildOnShard1 {
		t.Errorf("soundboard requests by shard = %v", got)
	}

	// The presence goes to every shard.
	if err := s.UpdatePresence(&discord.PresenceUpdate{Status: discord.StatusIdle}); err != nil {
		t.Fatal(err)
	}
	shards := []int{f.command(t).shard, f.command(t).shard}
	slices.Sort(shards)
	if !slices.Equal(shards, []int{0, 1}) {
		t.Errorf("presence went to shards %v, want both", shards)
	}
}

func TestGatewayCommandsForAShardNotConnected(t *testing.T) {
	f := newShardFake()
	s, _ := newFakeGateway(t, f.script(t))
	s.TotalShards, s.Shards = 2, []int{0}

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	err := s.UpdateVoiceState(guildOnShard1, "c1", false, false)
	if !errors.Is(err, ErrNotConnected) || !strings.Contains(err.Error(), "shard 1") {
		t.Errorf("UpdateVoiceState for another process's shard = %v", err)
	}
	if err := s.UpdateVoiceState(guildOnShard0, "c1", false, false); err != nil {
		t.Errorf("UpdateVoiceState for the shard connected = %v", err)
	}
}

func TestFatalCloseOnOneShardStopsEveryShard(t *testing.T) {
	checked := make(chan struct{})
	f := newShardFake()
	f.afterReady = func(c *fakeConn, shard, _ int) {
		if shard == 1 {
			<-checked
			c.closeWith(4014)
		}
	}
	s, g := newFakeGateway(t, repeat(f.script(t), 2)...)
	g.shards.Store(2)

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()
	close(checked)

	waitFor(t, s.Done(), "the gateway to stop")

	var closeErr *GatewayCloseError
	if err := s.Err(); !errors.As(err, &closeErr) || closeErr.Code != 4014 {
		t.Fatalf("Err = %v, want close code 4014", err)
	}

	ended := map[int]bool{}
	for range 2 {
		select {
		case shard := <-f.ended:
			ended[shard] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("connections closed for shards %v, want both", ended)
		}
	}
	if n := g.conns.Load(); n != 2 {
		t.Errorf("connections = %d, want no reconnect", n)
	}
}

func TestOpenFailsWhenAShardIsRefused(t *testing.T) {
	readied := make(chan struct{})
	f := newShardFake()
	f.afterReady = func(*fakeConn, int, int) { close(readied) }
	s, g := newFakeGateway(t,
		f.script(t),
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			<-readied // the other shard is ready, and must be closed again
			c.closeWith(4014)
			c.waitClosed()
		},
	)
	g.shards.Store(2)

	var closeErr *GatewayCloseError
	if err := s.Open(t.Context()); !errors.As(err, &closeErr) || closeErr.Code != 4014 {
		t.Fatalf("Open = %v, want close code 4014", err)
	}
	if s.Done() != nil {
		t.Error("Done is set after a failed Open")
	}
	select {
	case <-f.ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the shard that was ready stayed connected")
	}
}

func TestOpenTimeoutAllowsForEachIdentify(t *testing.T) {
	run := func(maxConcurrency int, ids ...int) *shardSet {
		info := &discord.GatewayBot{SessionStartLimit: discord.SessionStartLimit{MaxConcurrency: maxConcurrency}}
		return newShardSet(info, 16, ids)
	}

	cases := []struct {
		run   *shardSet
		waits int // Identifies that the slowest shard waits for
	}{
		{run(1, 0), 0},
		{run(1, 0, 1, 2, 3), 3},
		{run(2, 0, 1, 2, 3), 1},
		{run(16, 0, 1, 2, 3), 0},
		{run(2, 1, 3, 5), 2},
	}
	for _, tc := range cases {
		if got, want := tc.run.openTimeout(), gatewayOpenTimeout+time.Duration(tc.waits)*identifyInterval; got != want {
			t.Errorf("openTimeout of %d shards = %s, want %s", len(tc.run.shards), got, want)
		}
	}
}

func TestShardOfBeforeOpen(t *testing.T) {
	s := New("Bot token")
	if got := s.ShardOf(guildOnShard1); got != 0 {
		t.Errorf("ShardOf without shards = %d, want 0", got)
	}
	s.TotalShards = 2
	if got := s.ShardOf(guildOnShard1); got != 1 {
		t.Errorf("ShardOf with 2 shards = %d, want 1", got)
	}
}

func TestGatewayCommandsRefuseGuildIDsWithoutAShard(t *testing.T) {
	f := newShardFake()
	s, g := newFakeGateway(t, repeat(f.script(t), 2)...)
	g.shards.Store(2)

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	for name, err := range map[string]error{
		"UpdateVoiceState":        s.UpdateVoiceState("", "c1", false, false),
		"RequestGuildMembers":     s.RequestGuildMembers(discord.GuildMembersRequest{GuildID: "g1"}),
		"RequestSoundboardSounds": s.RequestSoundboardSounds(guildOnShard0, "g1"),
	} {
		if err == nil || !strings.Contains(err.Error(), "not a snowflake") || errors.Is(err, ErrNotConnected) {
			t.Errorf("%s with an invalid guild ID = %v", name, err)
		}
	}

	select {
	case c := <-f.commands:
		t.Errorf("sent op %d to shard %d for an invalid guild ID", c.op, c.shard)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestShardOfAfterAFailedOpen(t *testing.T) {
	f := newShardFake()
	s, g := newFakeGateway(t, f.script(t), func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.closeWith(4014)
		c.waitClosed()
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := s.ShardOf(guildOnShard1); got != 0 {
		t.Fatalf("ShardOf after connecting 1 shard = %d", got)
	}

	g.shards.Store(3) // not used: TotalShards wins
	s.TotalShards = 2
	if err := s.Open(t.Context()); err == nil {
		t.Fatal("second Open succeeded")
	}
	if got := s.ShardOf(guildOnShard1); got != 1 {
		t.Errorf("ShardOf after a failed Open = %d, want 1 from TotalShards", got)
	}
}

func TestShardingRequiredReshards(t *testing.T) {
	trigger := make(chan struct{})
	f := newShardFake()
	f.afterReady = func(c *fakeConn, _, count int) {
		if count == 1 {
			<-trigger
			c.closeWith(4011)
		}
	}
	s, g := newFakeGateway(t, repeat(f.script(t), 3)...)

	readies := make(chan struct{}, 3)
	s.OnReady(func(*Session, *discord.Ready) { readies <- struct{}{} })

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	select {
	case shard := <-f.identified:
		if shard != nil {
			t.Fatalf("first Identify has shard %v, want none", shard)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first Identify")
	}

	g.shards.Store(2)
	close(trigger)

	if got, want := f.identifiedShards(t, 2), [][2]int{{0, 2}, {1, 2}}; !slices.Equal(got, want) {
		t.Fatalf("identified shards after 4011 = %v, want %v", got, want)
	}
	for range 3 {
		select {
		case <-readies:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the new shards to be ready")
		}
	}

	select {
	case <-s.Done():
		t.Fatalf("the session stopped: %v", s.Err())
	default:
	}
	if got := s.ShardOf(guildOnShard1); got != 1 {
		t.Errorf("ShardOf after resharding = %d, want 1 of 2", got)
	}
	if err := s.UpdateVoiceState(guildOnShard1, "c1", false, false); err != nil {
		t.Fatal(err)
	}
	if c := f.command(t); c.shard != 1 || c.op != opVoiceStateUpdate {
		t.Errorf("voice state update went to shard %d as op %d", c.shard, c.op)
	}
}

func TestShardingRequiredStopsAChosenShardCount(t *testing.T) {
	f := newShardFake()
	f.afterReady = func(c *fakeConn, _, _ int) { c.closeWith(4011) }
	s, g := newFakeGateway(t, f.script(t))
	s.TotalShards = 1

	// The shard closes right after READY, so Open may or may not see it.
	if err := s.Open(t.Context()); err != nil {
		if !isShardingRequired(err) {
			t.Fatalf("Open = %v", err)
		}
		return
	}
	defer s.Close()

	waitFor(t, s.Done(), "the gateway to stop")
	if err := s.Err(); !isShardingRequired(err) {
		t.Errorf("Err = %v, want close code 4011", err)
	}
	if n := g.conns.Load(); n != 1 {
		t.Errorf("connections = %d, want no resharding with TotalShards set", n)
	}
}

func TestShardsIdentifyingAgainShareAGatewayFetch(t *testing.T) {
	var invalidated atomic.Int32
	f := newShardFake()
	f.afterReady = func(c *fakeConn, _, _ int) {
		if invalidated.Add(1) <= 2 {
			c.send(opInvalidSession, false, 0, "")
		}
	}
	s, g := newFakeGateway(t, repeat(f.script(t), 4)...)
	g.shards.Store(2)

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	f.identifiedShards(t, 4) // both shards, twice

	if n := g.starts.Load(); n != 2 {
		t.Errorf("GET /gateway/bot = %d times, want one for Open and one for both shards identifying again", n)
	}
}

func TestWaitIdentifyKeepsOtherBucketsQueuedSlots(t *testing.T) {
	s := New("Bot identify-slots-test")
	own, other := s.Token+"|0", s.Token+"|1"
	t.Cleanup(func() {
		identifySlots.mu.Lock()
		delete(identifySlots.next, own)
		delete(identifySlots.next, other)
		identifySlots.mu.Unlock()
	})

	// Bucket 1 has a slot coming up before bucket 0's next one.
	now := time.Now()
	identifySlots.mu.Lock()
	identifySlots.next[own] = now.Add(5 * time.Second)
	identifySlots.next[other] = now.Add(2 * time.Second)
	identifySlots.mu.Unlock()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // the slot is taken without waiting for it
	_ = s.waitIdentify(ctx, &shard{id: 0, maxConcurrency: 2})

	identifySlots.mu.Lock()
	_, kept := identifySlots.next[other]
	identifySlots.mu.Unlock()
	if !kept {
		t.Error("the queued slot of another bucket was dropped")
	}
}
