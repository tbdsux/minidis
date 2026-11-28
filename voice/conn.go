// Package voice connects to a Discord voice server and sends Opus audio:
// voice gateway v8, IP discovery, RTP over UDP with aead_aes256_gcm_rtpsize
// or aead_xchacha20_poly1305_rtpsize, and DAVE end-to-end encryption through
// a godave.Session (dave-go, pure Go, by default).
//
// It only sends audio. Receiving is not implemented.
package voice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/godave"
	"github.com/gorilla/websocket"
	"github.com/thomas-vilte/dave-go/session"
)

// Voice gateway opcodes.
const (
	opIdentify           = 0
	opSelectProtocol     = 1
	opReady              = 2
	opHeartbeat          = 3
	opSessionDescription = 4
	opSpeaking           = 5
	opHeartbeatACK       = 6
	opHello              = 8
	opClientsConnect     = 11
	opClientDisconnect   = 13

	opDavePrepareTransition    = 21
	opDaveExecuteTransition    = 22
	opDaveTransitionReady      = 23
	opDavePrepareEpoch         = 24
	opDaveExternalSender       = 25
	opDaveKeyPackage           = 26
	opDaveProposals            = 27
	opDaveCommitWelcome        = 28
	opDaveAnnounceCommit       = 29
	opDaveWelcome              = 30
	opDaveInvalidCommitWelcome = 31
)

// ErrNotReady is returned by WriteOpus while the DAVE handshake is still
// running. Hold the frame and retry it on the next tick.
var ErrNotReady = errors.New("voice: E2EE session not ready")

// Params are what Discord gave the bot for this voice connection: SessionID
// from its VOICE_STATE_UPDATE, Token and Endpoint from VOICE_SERVER_UPDATE.
type Params struct {
	GuildID, ChannelID, UserID string
	SessionID, Token, Endpoint string

	// Dave creates the E2EE session. Nil means dave-go.
	Dave   godave.SessionCreateFunc
	Logger *slog.Logger
}

// CloseError is the voice gateway's close frame.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("voice: gateway closed %d: %s", e.Code, e.Reason)
}

// gatewayURL is a var so tests can point it at a plain ws:// server.
var gatewayURL = func(endpoint string) string { return "wss://" + endpoint + "/?v=8" }

// Conn is one voice connection. WriteOpus must be called from one goroutine
// every 20ms; the other methods are safe to call from any goroutine.
type Conn struct {
	log  *slog.Logger
	ws   *websocket.Conn
	wsMu sync.Mutex
	udp  net.Conn
	dave godave.Session

	ssrc    uint32
	seq     uint16
	ts      uint32
	sealer  *sealer
	packet  []byte
	daveBuf []byte

	seqAck atomic.Int64
	ping   atomic.Int64 // ms, -1 until the first ACK

	ready     chan readyData
	described chan sessionDescription
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

type readyData struct {
	SSRC  uint32   `json:"ssrc"`
	IP    string   `json:"ip"`
	Port  int      `json:"port"`
	Modes []string `json:"modes"`
}

type sessionDescription struct {
	Mode                string `json:"mode"`
	SecretKey           []int  `json:"secret_key"`
	DaveProtocolVersion uint16 `json:"dave_protocol_version"`
}

type wsMessage struct {
	Op  int             `json:"op"`
	D   json.RawMessage `json:"d"`
	Seq *int64          `json:"seq,omitempty"`
}

// Dial connects to the voice server and returns once audio can be sent
// (after Session Description). ctx bounds the handshake only.
func Dial(ctx context.Context, p Params) (*Conn, error) {
	userID, err := strconv.ParseUint(p.UserID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("voice: bad user ID %q", p.UserID)
	}
	channelID, err := strconv.ParseUint(p.ChannelID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("voice: bad channel ID %q", p.ChannelID)
	}
	if p.Logger == nil {
		p.Logger = slog.Default()
	}
	if p.Dave == nil {
		p.Dave = session.CreateFunc(session.WithLogger(p.Logger))
	}

	ws, _, err := websocket.DefaultDialer.DialContext(ctx, gatewayURL(p.Endpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("voice: dial gateway: %w", err)
	}
	c := &Conn{
		log:       p.Logger.With("guild_id", p.GuildID),
		ws:        ws,
		ready:     make(chan readyData, 1),
		described: make(chan sessionDescription, 1),
		done:      make(chan struct{}),
	}
	c.seqAck.Store(-1)
	c.ping.Store(-1)
	c.dave = p.Dave(c.log, godave.UserID(strconv.FormatUint(userID, 10)), callbacks{c})
	c.dave.SetChannelID(godave.ChannelID(channelID))

	fail := func(err error) (*Conn, error) {
		c.shutdown(err)
		return nil, err
	}

	go c.readLoop()

	if err := c.sendJSON(opIdentify, map[string]any{
		"server_id":                 p.GuildID,
		"user_id":                   p.UserID,
		"session_id":                p.SessionID,
		"token":                     p.Token,
		"max_dave_protocol_version": c.dave.MaxSupportedProtocolVersion(),
	}); err != nil {
		return fail(err)
	}

	var rd readyData
	select {
	case rd = <-c.ready:
	case <-c.done:
		return nil, c.err
	case <-ctx.Done():
		return fail(ctx.Err())
	}
	c.ssrc = rd.SSRC

	var d net.Dialer
	if c.udp, err = d.DialContext(ctx, "udp", net.JoinHostPort(rd.IP, strconv.Itoa(rd.Port))); err != nil {
		return fail(fmt.Errorf("voice: dial udp: %w", err))
	}
	if dl, ok := ctx.Deadline(); ok {
		c.udp.SetReadDeadline(dl)
	}
	ip, port, err := discoverIP(c.udp, c.ssrc)
	if err != nil {
		return fail(fmt.Errorf("voice: ip discovery: %w", err))
	}
	c.udp.SetReadDeadline(time.Time{})

	mode := ModeXChaCha20
	if slices.Contains(rd.Modes, ModeAES256GCM) {
		mode = ModeAES256GCM
	}
	if err := c.sendJSON(opSelectProtocol, map[string]any{
		"protocol": "udp",
		"data":     map[string]any{"address": ip, "port": port, "mode": mode},
	}); err != nil {
		return fail(err)
	}

	var sd sessionDescription
	select {
	case sd = <-c.described:
	case <-c.done:
		return nil, c.err
	case <-ctx.Done():
		return fail(ctx.Err())
	}
	key := make([]byte, len(sd.SecretKey))
	for i, b := range sd.SecretKey {
		key[i] = byte(b)
	}
	if c.sealer, err = newSealer(sd.Mode, key); err != nil {
		return fail(err)
	}
	c.dave.AssignSsrcToCodec(c.ssrc, godave.CodecOpus)
	c.dave.OnSelectProtocolAck(sd.DaveProtocolVersion)
	return c, nil
}

// Speaking sets the speaking flag. Discord needs it before the first frame.
func (c *Conn) Speaking(on bool) error {
	flag := 0
	if on {
		flag = 1 // microphone
	}
	return c.sendJSON(opSpeaking, map[string]any{"speaking": flag, "delay": 0, "ssrc": c.ssrc})
}

// WriteOpus sends one 20ms Opus frame.
func (c *Conn) WriteOpus(frame []byte) error {
	select {
	case <-c.done:
		return c.err
	default:
	}
	if c.holdFrames() {
		return ErrNotReady
	}
	if n := c.dave.MaxEncryptedFrameSize(len(frame)); cap(c.daveBuf) < n {
		c.daveBuf = make([]byte, n)
	}
	n, err := c.dave.Encrypt(c.ssrc, frame, c.daveBuf[:cap(c.daveBuf)])
	if err != nil {
		return fmt.Errorf("voice: dave encrypt: %w", err)
	}

	var header [rtpHeaderSize]byte
	rtpHeader(header[:], c.seq, c.ts, c.ssrc)
	c.seq++
	c.ts += FrameSamples
	c.packet = c.sealer.seal(c.packet[:0], header[:], c.daveBuf[:n])
	_, err = c.udp.Write(c.packet)
	return err
}

// holdFrames reports a DAVE handshake in progress. dave-go's Ready stays
// false on channels without E2EE, so prefer its ShouldHoldFrames.
func (c *Conn) holdFrames() bool {
	if h, ok := c.dave.(interface{ ShouldHoldFrames() bool }); ok {
		return h.ShouldHoldFrames()
	}
	return !c.dave.Ready()
}

// Ping is the last gateway heartbeat round trip in ms, or -1.
func (c *Conn) Ping() int64 { return c.ping.Load() }

// Done is closed when the connection ends; Err then says why (a *CloseError
// for a gateway close frame).
func (c *Conn) Done() <-chan struct{} { return c.done }

// Err is the reason the connection ended, valid once Done is closed.
func (c *Conn) Err() error { return c.err }

// Close ends the connection.
func (c *Conn) Close() error {
	c.wsMu.Lock()
	c.ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	c.wsMu.Unlock()
	c.shutdown(&CloseError{Code: websocket.CloseNormalClosure, Reason: "closed by client"})
	return nil
}

func (c *Conn) shutdown(err error) {
	c.closeOnce.Do(func() {
		c.err = err
		close(c.done)
		c.ws.Close()
		if c.udp != nil {
			c.udp.Close()
		}
		if cl, ok := c.dave.(interface{ Close() error }); ok {
			cl.Close()
		}
	})
}

func (c *Conn) readLoop() {
	for {
		kind, data, err := c.ws.ReadMessage()
		if err != nil {
			if ce, ok := errors.AsType[*websocket.CloseError](err); ok {
				err = &CloseError{Code: ce.Code, Reason: ce.Text}
			}
			c.shutdown(err)
			return
		}
		if kind == websocket.BinaryMessage {
			c.handleBinary(data)
			continue
		}
		var m wsMessage
		if err := json.Unmarshal(data, &m); err != nil {
			c.log.Warn("voice: bad gateway message", "error", err)
			continue
		}
		if m.Seq != nil {
			c.seqAck.Store(*m.Seq)
		}
		c.handleJSON(m)
	}
}

func (c *Conn) handleJSON(m wsMessage) {
	var d struct {
		HeartbeatInterval float64  `json:"heartbeat_interval"`
		T                 int64    `json:"t"`
		UserIDs           []string `json:"user_ids"`
		UserID            string   `json:"user_id"`
		TransitionID      uint16   `json:"transition_id"`
		ProtocolVersion   uint16   `json:"protocol_version"`
		Epoch             int      `json:"epoch"`
	}
	switch m.Op {
	case opReady:
		var rd readyData
		if json.Unmarshal(m.D, &rd) == nil {
			select {
			case c.ready <- rd:
			default:
			}
		}
		return
	case opSessionDescription:
		var sd sessionDescription
		if json.Unmarshal(m.D, &sd) == nil {
			select {
			case c.described <- sd:
			default:
			}
		}
		return
	}
	json.Unmarshal(m.D, &d)
	switch m.Op {
	case opHello:
		go c.heartbeat(time.Duration(d.HeartbeatInterval * float64(time.Millisecond)))
	case opHeartbeatACK:
		c.ping.Store(time.Now().UnixMilli() - d.T)
	case opClientsConnect:
		for _, id := range d.UserIDs {
			c.dave.AddUser(godave.UserID(id))
		}
	case opClientDisconnect:
		c.dave.RemoveUser(godave.UserID(d.UserID))
	case opDavePrepareTransition:
		c.dave.OnDavePrepareTransition(d.TransitionID, d.ProtocolVersion)
	case opDaveExecuteTransition:
		c.dave.OnDaveExecuteTransition(d.TransitionID)
	case opDavePrepareEpoch:
		c.dave.OnDavePrepareEpoch(d.Epoch, d.ProtocolVersion)
	}
}

// handleBinary dispatches server DAVE frames: [seq u16][op u8][payload].
func (c *Conn) handleBinary(b []byte) {
	if len(b) < 3 {
		return
	}
	c.seqAck.Store(int64(binary.BigEndian.Uint16(b)))
	op, payload := b[2], b[3:]
	switch op {
	case opDaveExternalSender:
		c.dave.OnDaveMLSExternalSenderPackage(payload)
	case opDaveProposals:
		c.dave.OnDaveMLSProposals(payload)
	case opDaveAnnounceCommit, opDaveWelcome:
		if len(payload) < 2 {
			return
		}
		id := binary.BigEndian.Uint16(payload)
		if op == opDaveAnnounceCommit {
			c.dave.OnDaveMLSPrepareCommitTransition(id, payload[2:])
		} else {
			c.dave.OnDaveMLSWelcome(id, payload[2:])
		}
	}
}

func (c *Conn) heartbeat(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := c.sendJSON(opHeartbeat, map[string]any{
			"t": time.Now().UnixMilli(), "seq_ack": c.seqAck.Load(),
		}); err != nil {
			return
		}
		select {
		case <-t.C:
		case <-c.done:
			return
		}
	}
}

func (c *Conn) sendJSON(op int, d any) error {
	b, err := json.Marshal(map[string]any{"op": op, "d": d})
	if err != nil {
		return err
	}
	return c.write(websocket.TextMessage, b)
}

func (c *Conn) write(kind int, b []byte) error {
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	return c.ws.WriteMessage(kind, b)
}

// callbacks is the godave.Callbacks the DAVE session sends through.
type callbacks struct{ c *Conn }

func (cb callbacks) SendMLSKeyPackage(pkg []byte) error {
	return cb.c.write(websocket.BinaryMessage, append([]byte{opDaveKeyPackage}, pkg...))
}

func (cb callbacks) SendMLSCommitWelcome(msg []byte) error {
	return cb.c.write(websocket.BinaryMessage, append([]byte{opDaveCommitWelcome}, msg...))
}

func (cb callbacks) SendReadyForTransition(id uint16) error {
	return cb.c.sendJSON(opDaveTransitionReady, map[string]any{"transition_id": id})
}

func (cb callbacks) SendInvalidCommitWelcome(id uint16) error {
	return cb.c.sendJSON(opDaveInvalidCommitWelcome, map[string]any{"transition_id": id})
}
