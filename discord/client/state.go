package client

import (
	"encoding/json"
	"log"
	"maps"
	"runtime/debug"
	"slices"
	"strconv"
	"sync"

	"github.com/tbdsux/minidis/discord"
)

// StateOptions says what a State keeps beyond what it always does: guilds
// with their roles, emojis and stickers, channels, active threads, stage
// instances, scheduled events, soundboard sounds, and the bot's own member.
type StateOptions struct {
	// Members keeps the members of guilds, and asks for the privileged
	// discord.IntentsGuildMembers intent. Discord sends the members of large
	// guilds only as they become active; RequestGuildMembers gets the rest,
	// and the chunks it brings are kept too. Without Members, only the bot's
	// own member is kept, which is enough for its permissions.
	Members bool

	// KeepMember, if set, decides which members are kept, and a member is
	// dropped once it no longer returns true, such as after losing a role.
	// The bot's own member is always kept. Without Members, it chooses from
	// the members that Discord sends without the privileged intent: those
	// of messages, reactions, typing, interactions, voice states and
	// RequestGuildMembers.
	//
	// It runs on the gateway's goroutine, after the event is applied and
	// before its handlers start, so it may call the State's methods but must
	// be quick: events wait for it.
	KeepMember func(guildID string, m *discord.Member) bool

	// ThreadMembers keeps who is in each active thread, and asks for the
	// privileged discord.IntentsGuildMembers, without which Discord only
	// reports the bot. Discord sends who joins and leaves a thread rather
	// than a list of its members, so a thread has those who joined it while
	// the bot was connected; Session.ThreadMembers lists them all.
	ThreadMembers bool

	// VoiceStates keeps who is connected to voice channels.
	VoiceStates bool

	// Presences keeps the status and activities of the members who are not
	// offline, and asks for the privileged discord.IntentsGuildPresences.
	Presences bool

	// Messages is how many of the latest messages to keep in each channel. 0
	// keeps none. Their reactions and poll votes are counted as they come,
	// which asks for the intents of message reactions and polls.
	Messages int
}

// State is a cache of what the gateway has sent, kept up to date by the
// session it is set on, as Session.State, before Open:
//
//	s.State = client.NewState(client.StateOptions{Members: true})
//
// It always keeps guilds with their roles, emojis and stickers, their
// channels and active threads, stage instances, scheduled events and
// soundboard sounds, DM channels, and the bot's own member; and, as
// StateOptions says, members, thread members, voice states, presences and
// the latest messages. It is updated before an event's handlers start, so a
// handler sees the state with its event applied. It starts over with each
// new gateway session, as Discord sends every guild again then.
//
// What it returns is shared and must not be modified. A change to the state
// replaces a value rather than changing it, so a value that has been
// returned stays as it was. A Guild has its Roles, Emojis and Stickers, but
// not its channels, members and the other lists that Discord only sends in
// GUILD_CREATE: those are the State's other methods. A guild that Discord
// has announced but not sent, or that is in an outage, has only its ID and
// Unavailable.
//
// Bans, invites, webhooks, integrations, auto moderation rules and
// entitlements are not kept: Discord never sends them all on the gateway,
// so they would be incomplete.
//
// The methods of a nil State panic. The shards of a Session share its State,
// and so can several Sessions that split the shards of one bot.
type State struct {
	opts StateOptions

	mu            sync.RWMutex
	pending       []pendingMember // left to KeepMember by the update in progress
	self          string          // the bot's user ID
	guilds        map[string]*guildState
	channels      map[string]*discord.Channel // guild channels, active threads and DMs
	messages      map[string]*channelMessages // by channel ID
	dms           map[string]uint64           // DM channel IDs, by when they were last written in
	dmClock       uint64
	maxDMs        int
	threadMembers map[string]map[string]*discord.ThreadMember // by thread ID, then user ID
}

type guildState struct {
	guild     *discord.Guild
	channels  map[string]struct{} // the IDs of its channels and threads
	members   map[string]*discord.Member
	voice     map[string]*discord.VoiceState
	presences map[string]*discord.Presence
	stages    map[string]*discord.StageInstance
	events    map[string]*discord.GuildScheduledEvent
	sounds    map[string]*discord.SoundboardSound
}

type channelMessages struct {
	guildID string
	list    []*discord.Message // oldest first
}

// NewState creates a State that keeps what opts says.
func NewState(opts StateOptions) *State {
	return &State{
		opts:          opts,
		guilds:        map[string]*guildState{},
		channels:      map[string]*discord.Channel{},
		messages:      map[string]*channelMessages{},
		threadMembers: map[string]map[string]*discord.ThreadMember{},
		dms:           map[string]uint64{},
		maxDMs:        maxDMChannels,
	}
}

// maxDMChannels is how many DM channels the state keeps, with their
// messages: those most recently written in.
const maxDMChannels = 1000

// Intents returns the gateway intents that the state needs to be kept up to
// date. A Session identifies with them as well as its own Intents.
func (st *State) Intents() discord.Intent {
	intents := discord.IntentsGuilds | discord.IntentsGuildExpressions | discord.IntentsGuildScheduledEvents
	if st.opts.Members || st.opts.ThreadMembers {
		intents |= discord.IntentsGuildMembers
	}
	if st.opts.VoiceStates {
		intents |= discord.IntentsGuildVoiceStates
	}
	if st.opts.Presences {
		intents |= discord.IntentsGuildPresences
	}
	if st.opts.Messages > 0 {
		intents |= discord.IntentsGuildMessages | discord.IntentsDirectMessages |
			discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions |
			discord.IntentsGuildMessagePolls | discord.IntentsDirectMessagePolls
	}
	return intents
}

// lookup returns the value with the given ID in a map of a guild.
func lookup[V any](st *State, guildID, id string, of func(*guildState) map[string]V) V {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if gs := st.guilds[guildID]; gs != nil {
		return of(gs)[id]
	}
	var zero V
	return zero
}

// list returns the values of a map of a guild, in no particular order.
func list[V any](st *State, guildID string, of func(*guildState) map[string]V) []V {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if gs := st.guilds[guildID]; gs != nil {
		return slices.Collect(maps.Values(of(gs)))
	}
	return nil
}

func members(gs *guildState) map[string]*discord.Member                      { return gs.members }
func voiceStates(gs *guildState) map[string]*discord.VoiceState              { return gs.voice }
func presences(gs *guildState) map[string]*discord.Presence                  { return gs.presences }
func stageInstances(gs *guildState) map[string]*discord.StageInstance        { return gs.stages }
func scheduledEvents(gs *guildState) map[string]*discord.GuildScheduledEvent { return gs.events }
func soundboardSounds(gs *guildState) map[string]*discord.SoundboardSound    { return gs.sounds }

// Guild returns the guild with the given ID, or nil.
func (st *State) Guild(id string) *discord.Guild {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if gs := st.guilds[id]; gs != nil {
		return gs.guild
	}
	return nil
}

// Guilds returns the bot's guilds, in no particular order.
func (st *State) Guilds() []*discord.Guild {
	st.mu.RLock()
	defer st.mu.RUnlock()

	guilds := make([]*discord.Guild, 0, len(st.guilds))
	for _, gs := range st.guilds {
		guilds = append(guilds, gs.guild)
	}
	return guilds
}

// Channel returns the guild channel, active thread or DM channel with the
// given ID, or nil. A DM channel is there once someone sends the bot a
// message in it, or Discord sends it in a CHANNEL_CREATE event. The 1000 DM
// channels most recently written in are kept, with their messages.
func (st *State) Channel(id string) *discord.Channel {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return st.channels[id]
}

// GuildChannels returns the channels of a guild, without its threads, in no
// particular order.
func (st *State) GuildChannels(guildID string) []*discord.Channel {
	return st.guildChannels(guildID, false)
}

// GuildThreads returns the active threads of a guild that the bot can see,
// in no particular order.
func (st *State) GuildThreads(guildID string) []*discord.Channel {
	return st.guildChannels(guildID, true)
}

func (st *State) guildChannels(guildID string, threads bool) []*discord.Channel {
	st.mu.RLock()
	defer st.mu.RUnlock()

	gs := st.guilds[guildID]
	if gs == nil {
		return nil
	}
	var channels []*discord.Channel
	for id := range gs.channels {
		if c := st.channels[id]; c != nil && c.IsThread() == threads {
			channels = append(channels, c)
		}
	}
	return channels
}

// find returns the first element of s for which match is true, or nil.
func find[T any](s []*T, match func(*T) bool) *T {
	if i := slices.IndexFunc(s, match); i >= 0 {
		return s[i]
	}
	return nil
}

// Role returns a role of a guild, or nil.
func (st *State) Role(guildID, roleID string) *discord.Role {
	if g := st.Guild(guildID); g != nil {
		return find(g.Roles, func(r *discord.Role) bool { return r.ID == roleID })
	}
	return nil
}

// Emoji returns a custom emoji of a guild, or nil.
func (st *State) Emoji(guildID, emojiID string) *discord.Emoji {
	if g := st.Guild(guildID); g != nil {
		return find(g.Emojis, func(e *discord.Emoji) bool { return e.ID == emojiID })
	}
	return nil
}

// Sticker returns a sticker of a guild, or nil.
func (st *State) Sticker(guildID, stickerID string) *discord.Sticker {
	if g := st.Guild(guildID); g != nil {
		return find(g.Stickers, func(s *discord.Sticker) bool { return s.ID == stickerID })
	}
	return nil
}

// Member returns a member of a guild, or nil. Without StateOptions.Members
// or StateOptions.KeepMember, only the bot's own member is kept.
func (st *State) Member(guildID, userID string) *discord.Member {
	return lookup(st, guildID, userID, members)
}

// Members returns the members of a guild that the state has, in no
// particular order.
func (st *State) Members(guildID string) []*discord.Member {
	return list(st, guildID, members)
}

// VoiceState returns the voice state of a user who is connected to a voice
// channel of the guild, or nil. It needs StateOptions.VoiceStates.
func (st *State) VoiceState(guildID, userID string) *discord.VoiceState {
	return lookup(st, guildID, userID, voiceStates)
}

// VoiceStates returns the voice states of the users connected to the voice
// channels of a guild, in no particular order. It needs
// StateOptions.VoiceStates.
func (st *State) VoiceStates(guildID string) []*discord.VoiceState {
	return list(st, guildID, voiceStates)
}

// Presence returns the presence of a member who is not offline, or nil. It
// needs StateOptions.Presences.
func (st *State) Presence(guildID, userID string) *discord.Presence {
	return lookup(st, guildID, userID, presences)
}

// Presences returns the presences of the members of a guild who are not
// offline, in no particular order. It needs StateOptions.Presences.
func (st *State) Presences(guildID string) []*discord.Presence {
	return list(st, guildID, presences)
}

// StageInstance returns a live stage of a guild, or nil.
func (st *State) StageInstance(guildID, stageID string) *discord.StageInstance {
	return lookup(st, guildID, stageID, stageInstances)
}

// StageInstances returns the live stages of a guild, in no particular order.
func (st *State) StageInstances(guildID string) []*discord.StageInstance {
	return list(st, guildID, stageInstances)
}

// ScheduledEvent returns a scheduled event of a guild, or nil. Its
// UserCount is not kept.
func (st *State) ScheduledEvent(guildID, eventID string) *discord.GuildScheduledEvent {
	return lookup(st, guildID, eventID, scheduledEvents)
}

// ScheduledEvents returns the scheduled events of a guild, in no particular
// order.
func (st *State) ScheduledEvents(guildID string) []*discord.GuildScheduledEvent {
	return list(st, guildID, scheduledEvents)
}

// SoundboardSound returns a soundboard sound of a guild, or nil.
func (st *State) SoundboardSound(guildID, soundID string) *discord.SoundboardSound {
	return lookup(st, guildID, soundID, soundboardSounds)
}

// SoundboardSounds returns the soundboard sounds of a guild, in no
// particular order.
func (st *State) SoundboardSounds(guildID string) []*discord.SoundboardSound {
	return list(st, guildID, soundboardSounds)
}

// ThreadMember returns a member of an active thread, or nil. It needs
// StateOptions.ThreadMembers.
func (st *State) ThreadMember(threadID, userID string) *discord.ThreadMember {
	st.mu.RLock()
	defer st.mu.RUnlock()

	return st.threadMembers[threadID][userID]
}

// ThreadMembers returns the members of an active thread that the state has,
// in no particular order. It needs StateOptions.ThreadMembers.
func (st *State) ThreadMembers(threadID string) []*discord.ThreadMember {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if tm := st.threadMembers[threadID]; tm != nil {
		return slices.Collect(maps.Values(tm))
	}
	return nil
}

// Message returns one of the latest messages of a channel, or nil. It needs
// StateOptions.Messages.
func (st *State) Message(channelID, messageID string) *discord.Message {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if cm := st.messages[channelID]; cm != nil {
		return find(cm.list, func(m *discord.Message) bool { return m.ID == messageID })
	}
	return nil
}

// Messages returns the latest messages of a channel, oldest first. It needs
// StateOptions.Messages.
func (st *State) Messages(channelID string) []*discord.Message {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if cm := st.messages[channelID]; cm != nil {
		return slices.Clone(cm.list)
	}
	return nil
}

// Permissions computes the permissions of a member in a guild channel or
// thread, with discord.MemberPermissions, from the guild, channel and member
// in the state. A thread has the permissions of the channel it is in. ok is
// false if one of them is missing, as members other than the bot are without
// StateOptions.Members:
//
//	perms, ok := s.State.Permissions(channelID, s.User().ID)
func (st *State) Permissions(channelID, userID string) (perms int64, ok bool) {
	st.mu.RLock()
	defer st.mu.RUnlock()

	c := st.channels[channelID]
	if c == nil || c.GuildID == "" {
		return 0, false
	}
	if c.IsThread() {
		if c = st.channels[c.ParentID]; c == nil {
			return 0, false
		}
	}
	gs := st.guilds[c.GuildID]
	if gs == nil || gs.guild.Unavailable {
		return 0, false
	}
	m := gs.members[userID]
	if m == nil {
		return 0, false
	}
	return discord.MemberPermissions(gs.guild, c, m), true
}

// stateUpdates are how each event changes the state.
var stateUpdates = map[string]func(*State, json.RawMessage) error{
	"READY":       stateUpdate((*State).ready),
	"USER_UPDATE": stateUpdate((*State).userUpdate),

	"GUILD_CREATE":          stateUpdate((*State).guildCreate),
	"GUILD_UPDATE":          stateUpdate((*State).guildUpdate),
	"GUILD_DELETE":          stateUpdate((*State).guildDelete),
	"GUILD_ROLE_CREATE":     stateUpdate(func(st *State, e *discord.GuildRoleCreate) { st.putRole(e.GuildID, e.Role) }),
	"GUILD_ROLE_UPDATE":     stateUpdate(func(st *State, e *discord.GuildRoleUpdate) { st.putRole(e.GuildID, e.Role) }),
	"GUILD_ROLE_DELETE":     stateUpdate((*State).roleDelete),
	"GUILD_EMOJIS_UPDATE":   stateUpdate((*State).emojisUpdate),
	"GUILD_STICKERS_UPDATE": stateUpdate((*State).stickersUpdate),

	"GUILD_MEMBER_ADD":    stateUpdate((*State).memberAdd),
	"GUILD_MEMBER_UPDATE": stateUpdate((*State).memberUpdate),
	"GUILD_MEMBER_REMOVE": stateUpdate((*State).memberRemove),
	"GUILD_MEMBERS_CHUNK": stateUpdate((*State).membersChunk),
	"PRESENCE_UPDATE":     stateUpdate((*State).presenceUpdate),
	"VOICE_STATE_UPDATE":  stateUpdate((*State).voiceStateUpdate),
	"TYPING_START": stateUpdate(func(st *State, e *discord.TypingStart) {
		st.eventMember(e.GuildID, e.Member)
	}),
	"INTERACTION_CREATE": stateUpdate((*State).interactionCreate),

	"CHANNEL_CREATE":        stateUpdate(func(st *State, e *discord.ChannelCreate) { st.channelUpdate(e.Channel) }),
	"CHANNEL_UPDATE":        stateUpdate(func(st *State, e *discord.ChannelUpdate) { st.channelUpdate(e.Channel) }),
	"CHANNEL_DELETE":        stateUpdate((*State).channelDelete),
	"CHANNEL_PINS_UPDATE":   stateUpdate((*State).pinsUpdate),
	"THREAD_CREATE":         stateUpdate(func(st *State, e *discord.ThreadCreate) { st.threadUpdate(e.Channel) }),
	"THREAD_UPDATE":         stateUpdate(func(st *State, e *discord.ThreadUpdate) { st.threadUpdate(e.Channel) }),
	"THREAD_DELETE":         stateUpdate((*State).threadDelete),
	"THREAD_LIST_SYNC":      stateUpdate((*State).threadListSync),
	"THREAD_MEMBER_UPDATE":  stateUpdate((*State).threadMemberUpdate),
	"THREAD_MEMBERS_UPDATE": stateUpdate((*State).threadMembersUpdate),

	"STAGE_INSTANCE_CREATE": stateUpdate(func(st *State, e *discord.StageInstanceCreate) { st.putStage(e.StageInstance) }),
	"STAGE_INSTANCE_UPDATE": stateUpdate(func(st *State, e *discord.StageInstanceUpdate) { st.putStage(e.StageInstance) }),
	"STAGE_INSTANCE_DELETE": stateUpdate((*State).stageDelete),

	"GUILD_SCHEDULED_EVENT_CREATE": stateUpdate(func(st *State, e *discord.GuildScheduledEventCreate) { st.putEvent(e.GuildScheduledEvent) }),
	"GUILD_SCHEDULED_EVENT_UPDATE": stateUpdate(func(st *State, e *discord.GuildScheduledEventUpdate) { st.putEvent(e.GuildScheduledEvent) }),
	"GUILD_SCHEDULED_EVENT_DELETE": stateUpdate((*State).eventDelete),

	"GUILD_SOUNDBOARD_SOUND_CREATE":  stateUpdate(func(st *State, e *discord.GuildSoundboardSoundCreate) { st.putSound(e.SoundboardSound) }),
	"GUILD_SOUNDBOARD_SOUND_UPDATE":  stateUpdate(func(st *State, e *discord.GuildSoundboardSoundUpdate) { st.putSound(e.SoundboardSound) }),
	"GUILD_SOUNDBOARD_SOUND_DELETE":  stateUpdate((*State).soundDelete),
	"GUILD_SOUNDBOARD_SOUNDS_UPDATE": stateUpdate((*State).soundsUpdate),
	"SOUNDBOARD_SOUNDS":              stateUpdate((*State).soundsList),

	"MESSAGE_CREATE": func(st *State, data json.RawMessage) error {
		if st.opts.Messages > 0 {
			return messageCreateFull(st, data)
		}
		return messageCreateHead(st, data)
	},
	"MESSAGE_UPDATE":      stateUpdate((*State).messageUpdate),
	"MESSAGE_DELETE":      stateUpdate(func(st *State, e *discord.MessageDelete) { st.deleteMessages(e.ChannelID, e.ID) }),
	"MESSAGE_DELETE_BULK": stateUpdate(func(st *State, e *discord.MessageDeleteBulk) { st.deleteMessages(e.ChannelID, e.IDs...) }),

	"MESSAGE_REACTION_ADD":          stateUpdate((*State).reactionAdd),
	"MESSAGE_REACTION_REMOVE":       stateUpdate((*State).reactionRemove),
	"MESSAGE_REACTION_REMOVE_ALL":   stateUpdate((*State).reactionRemoveAll),
	"MESSAGE_REACTION_REMOVE_EMOJI": stateUpdate((*State).reactionRemoveEmoji),
	"MESSAGE_POLL_VOTE_ADD": stateUpdate(func(st *State, e *discord.MessagePollVoteAdd) {
		st.pollVote(e.ChannelID, e.MessageID, e.AnswerID, e.UserID, 1)
	}),
	"MESSAGE_POLL_VOTE_REMOVE": stateUpdate(func(st *State, e *discord.MessagePollVoteRemove) {
		st.pollVote(e.ChannelID, e.MessageID, e.AnswerID, e.UserID, -1)
	}),
}

// A state that keeps no messages decodes only the part of a message it
// uses, as it sees every one.
var (
	messageCreateFull = stateUpdate((*State).messageCreate)
	messageCreateHead = stateUpdate(func(st *State, h *messageHead) {
		st.messageCreate(&discord.MessageCreate{Message: &discord.Message{
			ID: h.ID, ChannelID: h.ChannelID, GuildID: h.GuildID, Author: h.Author, Member: h.Member,
		}})
	})
)

// messageHead is the part of a message that a state without messages uses.
type messageHead struct {
	ID        string          `json:"id"`
	ChannelID string          `json:"channel_id"`
	GuildID   string          `json:"guild_id"`
	Author    *discord.User   `json:"author"`
	Member    *discord.Member `json:"member"`
}

// interactionHead is the part of an interaction that the state uses: the
// member who used it. Decoding only this skips the interaction's data.
type interactionHead struct {
	GuildID string          `json:"guild_id"`
	Member  *discord.Member `json:"member"`
}

// stateUpdate decodes an event into an E of the state's own, so that the
// state never shares a value with handlers, which may change theirs, and
// applies it with f.
func stateUpdate[E any](f func(*State, *E)) func(*State, json.RawMessage) error {
	return func(st *State, data json.RawMessage) error {
		e := new(E)
		if err := json.Unmarshal(data, e); err != nil {
			return err
		}
		st.update(func() { f(st, e) })
		return nil
	}
}

// pendingMember is a member whose fate is up to StateOptions.KeepMember.
type pendingMember struct {
	guildID  string
	member   *discord.Member
	ifAbsent bool // keep it only if the state has no member for the user
}

// update runs f holding the lock. Then it asks StateOptions.KeepMember about
// the members f left to it, without the lock, so that KeepMember may call
// the State's methods, and stores its answers.
func (st *State) update(f func()) {
	var pending []pendingMember
	func() {
		st.mu.Lock()
		defer func() {
			pending, st.pending = st.pending, nil
			st.mu.Unlock()
		}()
		f()
	}()
	if len(pending) == 0 {
		return
	}

	keep := make([]bool, len(pending))
	for i, p := range pending {
		keep[i] = st.opts.KeepMember(p.guildID, p.member)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	for i, p := range pending {
		gs := st.available(p.guildID)
		id := p.member.User.ID
		if gs == nil || (p.ifAbsent && gs.members[id] != nil) {
			continue
		}
		if keep[i] {
			gs.members[id] = p.member
		} else {
			delete(gs.members, id)
		}
	}
}

// apply updates the state with a gateway event. A panic, such as in
// StateOptions.KeepMember, is logged rather than stopping the gateway.
func (st *State) apply(event string, data json.RawMessage) {
	update := stateUpdates[event]
	if update == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("discord: state: panic while applying %s: %v\n%s", event, r, debug.Stack())
		}
	}()
	if err := update(st, data); err != nil {
		log.Printf("discord: state: invalid %s event: %v", event, err)
	}
}

// The methods below hold st.mu. The values they are given were decoded for
// the state alone, so they may change them until they are stored, but never
// a value that is stored: they replace it with a changed copy.

// ready starts a new gateway session over: the guilds of the shard that
// sent it are forgotten, and Discord sends each again.
func (st *State) ready(e *discord.Ready) {
	if e.User != nil {
		st.self = e.User.ID
	}
	shard, count := 0, 1
	if e.Shard != nil {
		shard, count = e.Shard[0], e.Shard[1]
	}

	for id := range st.guilds {
		if shardOf(id, count) == shard {
			st.removeGuild(id)
		}
	}
	// DMs come to shard 0.
	if shard == 0 {
		for id, c := range st.channels {
			if c.GuildID == "" {
				st.forgetChannel(id)
			}
		}
		for id, cm := range st.messages {
			if cm.guildID == "" {
				delete(st.messages, id)
			}
		}
		clear(st.dms)
	}

	for _, g := range e.Guilds {
		st.guilds[g.ID] = newGuildState(&discord.Guild{ID: g.ID, Unavailable: true})
	}
}

// shardOf returns the shard that receives the events of a guild.
func shardOf(guildID string, count int) int {
	if count <= 1 {
		return 0
	}
	id, _ := strconv.ParseUint(guildID, 10, 64)
	return int((id >> 22) % uint64(count))
}

// userUpdate gives the bot's own members its new user.
func (st *State) userUpdate(e *discord.UserUpdate) {
	if e.User == nil || e.ID != st.self {
		return
	}
	for _, gs := range st.guilds {
		if m := gs.members[st.self]; m != nil {
			n := *m
			n.User = e.User
			gs.members[st.self] = &n
		}
	}
}

func newGuildState(g *discord.Guild) *guildState {
	return &guildState{
		guild:     g,
		channels:  map[string]struct{}{},
		members:   map[string]*discord.Member{},
		voice:     map[string]*discord.VoiceState{},
		presences: map[string]*discord.Presence{},
		stages:    map[string]*discord.StageInstance{},
		events:    map[string]*discord.GuildScheduledEvent{},
		sounds:    map[string]*discord.SoundboardSound{},
	}
}

// available returns the state of a guild that is not unavailable, or nil.
func (st *State) available(guildID string) *guildState {
	if gs := st.guilds[guildID]; gs != nil && !gs.guild.Unavailable {
		return gs
	}
	return nil
}

// guildOnly leaves out the lists of a guild that the state keeps apart.
func guildOnly(g *discord.Guild) *discord.Guild {
	g.VoiceStates, g.Members, g.Channels, g.Threads, g.Presences = nil, nil, nil, nil, nil
	g.StageInstances, g.GuildScheduledEvents, g.SoundboardSounds = nil, nil, nil
	return g
}

func (st *State) removeGuild(id string) {
	gs := st.guilds[id]
	if gs == nil {
		return
	}
	for cid := range gs.channels {
		st.forgetChannel(cid)
	}
	for cid, cm := range st.messages {
		if cm.guildID == id {
			delete(st.messages, cid)
		}
	}
	delete(st.guilds, id)
}

func (st *State) guildCreate(e *discord.GuildCreate) {
	g := e.Guild
	if g == nil || g.ID == "" {
		return
	}
	st.removeGuild(g.ID)

	if g.Unavailable {
		st.guilds[g.ID] = newGuildState(&discord.Guild{ID: g.ID, Unavailable: true})
		return
	}

	channels, threads, members := g.Channels, g.Threads, g.Members
	voice, presences := g.VoiceStates, g.Presences
	stages, events, sounds := g.StageInstances, g.GuildScheduledEvents, g.SoundboardSounds
	gs := newGuildState(guildOnly(g))
	st.guilds[g.ID] = gs

	for _, c := range channels {
		c.GuildID = g.ID // not sent inside a guild
		st.putChannel(gs, c)
	}
	for _, t := range threads {
		t.GuildID = g.ID
		st.putThread(gs, t)
	}
	for _, m := range members {
		st.putMember(gs, m)
	}
	if st.opts.VoiceStates {
		for _, v := range voice {
			v.GuildID = g.ID
			if v.ChannelID != "" {
				gs.voice[v.UserID] = v
			}
		}
	}
	if st.opts.Presences {
		for _, p := range presences {
			p.GuildID = g.ID
			putPresence(gs, p)
		}
	}
	for _, s := range stages {
		s.GuildID = g.ID
		gs.stages[s.ID] = s
	}
	for _, ev := range events {
		ev.GuildID, ev.UserCount = g.ID, 0
		gs.events[ev.ID] = ev
	}
	for _, s := range sounds {
		s.GuildID = g.ID
		gs.sounds[s.SoundID] = s
	}
}

// guildUpdate keeps what GUILD_UPDATE does not send.
func (st *State) guildUpdate(e *discord.GuildUpdate) {
	g := e.Guild
	if g == nil {
		return
	}
	gs := st.available(g.ID)
	if gs == nil {
		return
	}
	g = guildOnly(g)
	old := gs.guild
	g.JoinedAt, g.Large, g.MemberCount, g.Unavailable = old.JoinedAt, old.Large, old.MemberCount, false
	if g.Roles == nil {
		g.Roles = old.Roles
	}
	if g.Emojis == nil {
		g.Emojis = old.Emojis
	}
	if g.Stickers == nil {
		g.Stickers = old.Stickers
	}
	gs.guild = g
}

func (st *State) guildDelete(e *discord.GuildDelete) {
	st.removeGuild(e.ID)
	if e.Unavailable {
		st.guilds[e.ID] = newGuildState(&discord.Guild{ID: e.ID, Unavailable: true})
	}
}

// changeGuild replaces the guild with a copy that change has changed.
func (st *State) changeGuild(id string, change func(*discord.Guild)) {
	if gs := st.available(id); gs != nil {
		g := *gs.guild
		change(&g)
		gs.guild = &g
	}
}

func (st *State) putRole(guildID string, role *discord.Role) {
	if role == nil {
		return
	}
	st.changeGuild(guildID, func(g *discord.Guild) {
		roles := make([]*discord.Role, 0, len(g.Roles)+1)
		for _, r := range g.Roles {
			if r.ID != role.ID {
				roles = append(roles, r)
			}
		}
		g.Roles = append(roles, role)
	})
}

func (st *State) roleDelete(e *discord.GuildRoleDelete) {
	st.changeGuild(e.GuildID, func(g *discord.Guild) {
		g.Roles = slices.DeleteFunc(slices.Clone(g.Roles), func(r *discord.Role) bool { return r.ID == e.RoleID })
	})
}

func (st *State) emojisUpdate(e *discord.GuildEmojisUpdate) {
	st.changeGuild(e.GuildID, func(g *discord.Guild) { g.Emojis = e.Emojis })
}

func (st *State) stickersUpdate(e *discord.GuildStickersUpdate) {
	st.changeGuild(e.GuildID, func(g *discord.Guild) { g.Stickers = e.Stickers })
}

// putMember keeps a member, which must have its User, if it is the bot's or
// the options keep it, and otherwise drops the one the state has. A member
// that is up to StateOptions.KeepMember is left to update, which asks it
// without the lock.
func (st *State) putMember(gs *guildState, m *discord.Member) {
	st.putMemberIf(gs, m, false)
}

func (st *State) putMemberIf(gs *guildState, m *discord.Member, ifAbsent bool) {
	if m == nil || m.User == nil {
		return
	}
	id := m.User.ID
	switch {
	case id != st.self && st.opts.KeepMember != nil:
		st.pending = append(st.pending, pendingMember{gs.guild.ID, m, ifAbsent})
	case id != st.self && !st.opts.Members:
		delete(gs.members, id)
	default:
		gs.members[id] = m
	}
}

// eventMember keeps the member that came with an event, with its user.
func (st *State) eventMember(guildID string, m *discord.Member) {
	if gs := st.available(guildID); gs != nil {
		st.putMember(gs, m)
	}
}

// fetchedMember keeps a copy of a member fetched from the REST API, so that
// the caller may change theirs, unless the state has one already: that came
// from the gateway, and may be newer.
func (st *State) fetchedMember(guildID string, m *discord.Member) {
	if m == nil || m.User == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("discord: state: panic while keeping a fetched member: %v\n%s", r, debug.Stack())
		}
	}()

	c := *m
	u := *m.User
	c.User, c.Roles = &u, slices.Clone(m.Roles)
	st.update(func() {
		if gs := st.available(guildID); gs != nil && gs.members[u.ID] == nil {
			st.putMemberIf(gs, &c, true)
		}
	})
}

func (st *State) memberAdd(e *discord.GuildMemberAdd) {
	gs := st.available(e.GuildID)
	if gs == nil {
		return
	}
	st.putMember(gs, e.Member)
	st.changeGuild(e.GuildID, func(g *discord.Guild) { g.MemberCount++ })
}

// memberUpdate keeps whether the member is deafened and muted, which
// GUILD_MEMBER_UPDATE does not say.
func (st *State) memberUpdate(e *discord.GuildMemberUpdate) {
	gs := st.available(e.GuildID)
	if gs == nil || e.Member == nil || e.User == nil {
		return
	}
	if old := gs.members[e.User.ID]; old != nil {
		e.Deaf, e.Mute = old.Deaf, old.Mute
	}
	st.putMember(gs, e.Member)
}

func (st *State) memberRemove(e *discord.GuildMemberRemove) {
	gs := st.available(e.GuildID)
	if gs == nil || e.User == nil {
		return
	}
	delete(gs.members, e.User.ID)
	delete(gs.presences, e.User.ID)
	st.changeGuild(e.GuildID, func(g *discord.Guild) { g.MemberCount = max(g.MemberCount-1, 0) })
}

func (st *State) membersChunk(e *discord.GuildMembersChunk) {
	gs := st.available(e.GuildID)
	if gs == nil {
		return
	}
	for _, m := range e.Members {
		st.putMember(gs, m)
	}
	if st.opts.Presences {
		for _, p := range e.Presences {
			p.GuildID = e.GuildID
			putPresence(gs, p)
		}
	}
}

// interactionCreate keeps the member who used an interaction in a guild.
// Its Permissions are about the interaction's channel, so they are left out.
func (st *State) interactionCreate(e *interactionHead) {
	if e.Member == nil {
		return
	}
	e.Member.Permissions = 0
	st.eventMember(e.GuildID, e.Member)
}

func putPresence(gs *guildState, p *discord.Presence) {
	if p.User == nil {
		return
	}
	if p.Status == discord.StatusOffline {
		delete(gs.presences, p.User.ID)
		return
	}
	gs.presences[p.User.ID] = p
}

func (st *State) presenceUpdate(e *discord.PresenceUpdateEvent) {
	if !st.opts.Presences || e.Presence == nil {
		return
	}
	if gs := st.available(e.GuildID); gs != nil {
		putPresence(gs, e.Presence)
	}
}

func (st *State) voiceStateUpdate(e *discord.VoiceStateUpdate) {
	v := e.VoiceState
	if v == nil {
		return
	}
	gs := st.available(v.GuildID)
	if gs == nil {
		return
	}
	st.putMember(gs, v.Member)
	if !st.opts.VoiceStates {
		return
	}
	if v.ChannelID == "" {
		delete(gs.voice, v.UserID)
	} else {
		gs.voice[v.UserID] = v
	}
}

func (st *State) putChannel(gs *guildState, c *discord.Channel) {
	st.channels[c.ID] = c
	if gs != nil {
		gs.channels[c.ID] = struct{}{}
	}
}

// forgetChannel forgets a channel, its messages and, for a thread, its
// members.
func (st *State) forgetChannel(id string) {
	delete(st.channels, id)
	delete(st.messages, id)
	delete(st.threadMembers, id)
	delete(st.dms, id)
}

// touchDM records that a DM channel was written in, and forgets the one
// least recently written in once there are more than the state keeps.
func (st *State) touchDM(id string) {
	st.dmClock++
	st.dms[id] = st.dmClock
	if len(st.dms) <= st.maxDMs {
		return
	}
	oldest, at := "", st.dmClock
	for cid, t := range st.dms {
		if t < at {
			oldest, at = cid, t
		}
	}
	st.forgetChannel(oldest)
}

// removeChannel forgets a channel of a guild, and the threads in it.
func (st *State) removeChannel(guildID, id string) {
	st.forgetChannel(id)

	gs := st.guilds[guildID]
	if gs == nil {
		return
	}
	delete(gs.channels, id)
	for cid := range gs.channels {
		if c := st.channels[cid]; c != nil && c.IsThread() && c.ParentID == id {
			st.removeChannel(guildID, cid)
		}
	}
}

func (st *State) channelUpdate(c *discord.Channel) {
	if c == nil || c.ID == "" {
		return
	}
	if c.GuildID == "" {
		st.putChannel(nil, c)
		st.touchDM(c.ID)
		return
	}
	if gs := st.available(c.GuildID); gs != nil {
		st.putChannel(gs, c)
	}
}

func (st *State) channelDelete(e *discord.ChannelDelete) {
	if e.Channel != nil {
		st.removeChannel(e.GuildID, e.ID)
	}
}

// changeChannel replaces a channel with a copy that change has changed.
func (st *State) changeChannel(id string, change func(*discord.Channel)) {
	if c := st.channels[id]; c != nil {
		n := *c
		change(&n)
		st.channels[id] = &n
	}
}

func (st *State) pinsUpdate(e *discord.ChannelPinsUpdate) {
	st.changeChannel(e.ChannelID, func(c *discord.Channel) { c.LastPinTimestamp = e.LastPinTimestamp })
}

// putThread keeps a thread, and the bot's membership of it, which comes
// without the IDs it has elsewhere.
func (st *State) putThread(gs *guildState, t *discord.Channel) {
	st.putChannel(gs, t)
	if m := t.Member; m != nil {
		if m.ID == "" {
			m.ID = t.ID
		}
		if m.UserID == "" {
			m.UserID = st.self
		}
		st.putThreadMember(t.ID, m)
	}
}

// threadUpdate keeps active threads, and forgets those that are archived.
// THREAD_UPDATE does not say whether the bot is a member, so that is kept.
func (st *State) threadUpdate(c *discord.Channel) {
	if c == nil || c.ID == "" {
		return
	}
	if c.ThreadMetadata != nil && c.ThreadMetadata.Archived {
		st.removeChannel(c.GuildID, c.ID)
		return
	}
	gs := st.available(c.GuildID)
	if gs == nil {
		return
	}
	if old := st.channels[c.ID]; old != nil && c.Member == nil {
		c.Member = old.Member
		st.putChannel(gs, c)
		return
	}
	st.putThread(gs, c)
}

func (st *State) threadDelete(e *discord.ThreadDelete) {
	st.removeChannel(e.GuildID, e.ID)
}

// threadListSync replaces the threads of the channels it is about, or of
// the whole guild.
func (st *State) threadListSync(e *discord.ThreadListSync) {
	gs := st.available(e.GuildID)
	if gs == nil {
		return
	}
	for cid := range gs.channels {
		c := st.channels[cid]
		if c != nil && c.IsThread() && (len(e.ChannelIDs) == 0 || slices.Contains(e.ChannelIDs, c.ParentID)) {
			st.removeChannel(e.GuildID, cid)
		}
	}

	joined := map[string]*discord.ThreadMember{}
	for _, m := range e.Members {
		joined[m.ID] = m
	}
	for _, t := range e.Threads {
		t.GuildID = e.GuildID
		if m := joined[t.ID]; m != nil {
			t.Member = m
		}
		st.putThread(gs, t)
	}
}

// putThreadMember keeps a member of a thread that the state has.
func (st *State) putThreadMember(threadID string, m *discord.ThreadMember) {
	if !st.opts.ThreadMembers || m == nil || m.UserID == "" || st.channels[threadID] == nil {
		return
	}
	tm := st.threadMembers[threadID]
	if tm == nil {
		tm = map[string]*discord.ThreadMember{}
		st.threadMembers[threadID] = tm
	}
	tm[m.UserID] = m
}

func (st *State) threadMemberUpdate(e *discord.ThreadMemberUpdate) {
	if e.ThreadMember == nil {
		return
	}
	st.changeChannel(e.ID, func(c *discord.Channel) { c.Member = e.ThreadMember })
	st.putThreadMember(e.ID, e.ThreadMember)
}

func (st *State) threadMembersUpdate(e *discord.ThreadMembersUpdate) {
	st.changeChannel(e.ID, func(c *discord.Channel) {
		c.MemberCount = e.MemberCount
		for _, m := range e.AddedMembers {
			if m.UserID == st.self {
				c.Member = m
			}
		}
		if slices.Contains(e.RemovedMemberIDs, st.self) {
			c.Member = nil
		}
	})

	for _, m := range e.AddedMembers {
		if m.ID == "" {
			m.ID = e.ID
		}
		st.putThreadMember(e.ID, m)
		st.eventMember(e.GuildID, m.Member)
	}
	if tm := st.threadMembers[e.ID]; tm != nil {
		for _, id := range e.RemovedMemberIDs {
			delete(tm, id)
		}
	}
}

func (st *State) putStage(s *discord.StageInstance) {
	if s == nil {
		return
	}
	if gs := st.available(s.GuildID); gs != nil {
		gs.stages[s.ID] = s
	}
}

func (st *State) stageDelete(e *discord.StageInstanceDelete) {
	if s := e.StageInstance; s != nil {
		if gs := st.available(s.GuildID); gs != nil {
			delete(gs.stages, s.ID)
		}
	}
}

// putEvent keeps a scheduled event. Its user count is only sent when asked
// for, so it is not kept.
func (st *State) putEvent(ev *discord.GuildScheduledEvent) {
	if ev == nil {
		return
	}
	if gs := st.available(ev.GuildID); gs != nil {
		ev.UserCount = 0
		gs.events[ev.ID] = ev
	}
}

func (st *State) eventDelete(e *discord.GuildScheduledEventDelete) {
	if ev := e.GuildScheduledEvent; ev != nil {
		if gs := st.available(ev.GuildID); gs != nil {
			delete(gs.events, ev.ID)
		}
	}
}

func (st *State) putSound(s *discord.SoundboardSound) {
	if s == nil {
		return
	}
	if gs := st.available(s.GuildID); gs != nil {
		gs.sounds[s.SoundID] = s
	}
}

func (st *State) soundDelete(e *discord.GuildSoundboardSoundDelete) {
	if gs := st.available(e.GuildID); gs != nil {
		delete(gs.sounds, e.SoundID)
	}
}

// soundsUpdate keeps the sounds that changed together.
func (st *State) soundsUpdate(e *discord.GuildSoundboardSoundsUpdate) {
	for _, s := range e.SoundboardSounds {
		if s.GuildID == "" {
			s.GuildID = e.GuildID
		}
		st.putSound(s)
	}
}

// soundsList replaces the sounds of a guild with all of them, as sent for
// Session.RequestSoundboardSounds.
func (st *State) soundsList(e *discord.SoundboardSounds) {
	gs := st.available(e.GuildID)
	if gs == nil {
		return
	}
	clear(gs.sounds)
	for _, s := range e.SoundboardSounds {
		s.GuildID = e.GuildID
		gs.sounds[s.SoundID] = s
	}
}

func (st *State) messageCreate(e *discord.MessageCreate) {
	m := e.Message
	if m == nil || m.ChannelID == "" {
		return
	}
	st.changeChannel(m.ChannelID, func(c *discord.Channel) {
		c.LastMessageID = m.ID
		// Discord does not count the first message of a forum post, whose
		// ID is the thread's.
		if c.IsThread() && m.ID != c.ID {
			c.MessageCount++
			c.TotalMessageSent++
		}
	})

	if m.GuildID == "" {
		// A DM, in a channel Discord does not otherwise send.
		if st.channels[m.ChannelID] == nil && m.Author != nil && m.Author.ID != st.self {
			st.putChannel(nil, &discord.Channel{ID: m.ChannelID, Type: discord.ChannelTypeDM,
				LastMessageID: m.ID, Recipients: []*discord.User{m.Author}})
		}
		defer st.touchDM(m.ChannelID) // after its message is kept, so that it can be forgotten with it
	} else if m.Member != nil && m.Author != nil {
		// The member of a message comes without its user.
		m.Member.User = m.Author
		st.eventMember(m.GuildID, m.Member)
	}

	if st.opts.Messages <= 0 {
		return
	}
	cm := st.messages[m.ChannelID]
	if cm == nil {
		cm = &channelMessages{guildID: m.GuildID}
		st.messages[m.ChannelID] = cm
	}
	if len(cm.list) >= st.opts.Messages {
		n := copy(cm.list, cm.list[len(cm.list)-st.opts.Messages+1:])
		clear(cm.list[n:])
		cm.list = cm.list[:n]
	}
	cm.list = append(cm.list, m)
}

// messageUpdate replaces a message that the state has. An update without
// the author, which Discord used to send for embeds it added, is ignored.
// What an update leaves out is kept: the reactions and poll votes counted
// from their own events, and the author's member.
func (st *State) messageUpdate(e *discord.MessageUpdate) {
	m := e.Message
	if m == nil || m.Author == nil {
		return
	}
	cm := st.messages[m.ChannelID]
	if cm == nil {
		return
	}
	i := slices.IndexFunc(cm.list, func(old *discord.Message) bool { return old.ID == m.ID })
	if i < 0 {
		return
	}
	old := cm.list[i]
	if m.Reactions == nil {
		m.Reactions = old.Reactions
	}
	if m.Poll != nil && m.Poll.Results == nil && old.Poll != nil {
		m.Poll.Results = old.Poll.Results
	}
	if m.Member == nil {
		m.Member = old.Member
	}
	cm.list[i] = m
}

// deleteMessages forgets deleted messages, and takes them off the message
// count of a thread, which, unlike TotalMessageSent, does not count them.
func (st *State) deleteMessages(channelID string, ids ...string) {
	if c := st.channels[channelID]; c != nil && c.IsThread() {
		n := len(ids)
		if slices.Contains(ids, c.ID) {
			n--
		}
		if n > 0 {
			st.changeChannel(channelID, func(c *discord.Channel) { c.MessageCount = max(c.MessageCount-n, 0) })
		}
	}
	if cm := st.messages[channelID]; cm != nil {
		cm.list = slices.DeleteFunc(cm.list, func(m *discord.Message) bool { return slices.Contains(ids, m.ID) })
	}
}

// changeMessage replaces a message that the state has with a copy that
// change has changed.
func (st *State) changeMessage(channelID, messageID string, change func(*discord.Message)) {
	cm := st.messages[channelID]
	if cm == nil {
		return
	}
	if i := slices.IndexFunc(cm.list, func(m *discord.Message) bool { return m.ID == messageID }); i >= 0 {
		n := *cm.list[i]
		change(&n)
		cm.list[i] = &n
	}
}

// sameEmoji reports whether two emojis of reactions are the same: custom
// emojis by ID, and Unicode ones by name.
func sameEmoji(a, b *discord.Emoji) bool {
	if a == nil || b == nil {
		return false
	}
	if a.ID != "" || b.ID != "" {
		return a.ID == b.ID
	}
	return a.Name == b.Name
}

func (st *State) reactionAdd(e *discord.MessageReactionAdd) {
	st.eventMember(e.GuildID, e.Member)
	if e.Emoji == nil {
		return
	}
	st.changeMessage(e.ChannelID, e.MessageID, func(m *discord.Message) {
		reactions := slices.Clone(m.Reactions)
		i := slices.IndexFunc(reactions, func(r *discord.MessageReaction) bool { return sameEmoji(r.Emoji, e.Emoji) })
		var r discord.MessageReaction
		if i < 0 {
			r = discord.MessageReaction{Emoji: e.Emoji, BurstColors: e.BurstColors}
			reactions = append(reactions, nil)
			i = len(reactions) - 1
		} else {
			r = *reactions[i]
		}

		me := e.UserID == st.self
		r.Count++
		if e.Burst {
			r.CountDetails.Burst++
			r.MeBurst = r.MeBurst || me
		} else {
			r.CountDetails.Normal++
			r.Me = r.Me || me
		}
		reactions[i] = &r
		m.Reactions = reactions
	})
}

func (st *State) reactionRemove(e *discord.MessageReactionRemove) {
	if e.Emoji == nil {
		return
	}
	st.changeMessage(e.ChannelID, e.MessageID, func(m *discord.Message) {
		i := slices.IndexFunc(m.Reactions, func(r *discord.MessageReaction) bool { return sameEmoji(r.Emoji, e.Emoji) })
		if i < 0 {
			return
		}
		reactions := slices.Clone(m.Reactions)
		r := *reactions[i]

		me := e.UserID == st.self
		r.Count = max(r.Count-1, 0)
		if e.Burst {
			r.CountDetails.Burst = max(r.CountDetails.Burst-1, 0)
			r.MeBurst = r.MeBurst && !me
		} else {
			r.CountDetails.Normal = max(r.CountDetails.Normal-1, 0)
			r.Me = r.Me && !me
		}

		if r.Count == 0 {
			reactions = slices.Delete(reactions, i, i+1)
		} else {
			reactions[i] = &r
		}
		m.Reactions = reactions
	})
}

func (st *State) reactionRemoveAll(e *discord.MessageReactionRemoveAll) {
	st.changeMessage(e.ChannelID, e.MessageID, func(m *discord.Message) { m.Reactions = nil })
}

func (st *State) reactionRemoveEmoji(e *discord.MessageReactionRemoveEmoji) {
	st.changeMessage(e.ChannelID, e.MessageID, func(m *discord.Message) {
		m.Reactions = slices.DeleteFunc(slices.Clone(m.Reactions), func(r *discord.MessageReaction) bool { return sameEmoji(r.Emoji, e.Emoji) })
	})
}

// pollVote counts a vote, or with a negative delta takes one back, in the
// poll of a message that the state has.
func (st *State) pollVote(channelID, messageID string, answerID int, userID string, delta int) {
	st.changeMessage(channelID, messageID, func(m *discord.Message) {
		if m.Poll == nil {
			return
		}
		p := *m.Poll
		var results discord.PollResults
		if p.Results != nil {
			results = *p.Results
		}

		counts := slices.Clone(results.AnswerCounts)
		i := slices.IndexFunc(counts, func(c discord.PollAnswerCount) bool { return c.ID == answerID })
		if i < 0 {
			counts = append(counts, discord.PollAnswerCount{ID: answerID})
			i = len(counts) - 1
		}
		counts[i].Count = max(counts[i].Count+delta, 0)
		if userID == st.self {
			counts[i].MeVoted = delta > 0
		}

		results.AnswerCounts = counts
		p.Results = &results
		m.Poll = &p
	})
}
