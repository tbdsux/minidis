package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/godave"
	"github.com/gorilla/websocket"
)

// fakeDave records the DAVE calls the gateway dispatches.
type fakeDave struct {
	godave.Session // panics on anything not overridden below
	cb             godave.Callbacks
	calls          chan string
}

func (f *fakeDave) MaxSupportedProtocolVersion() int { return 1 }
func (f *fakeDave) Ready() bool                      { return true }
func (f *fakeDave) SetChannelID(id godave.ChannelID) { f.calls <- fmt.Sprint("channel ", id) }
func (f *fakeDave) AssignSsrcToCodec(ssrc uint32, _ godave.Codec) {
	f.calls <- fmt.Sprint("ssrc ", ssrc)
}
func (f *fakeDave) MaxEncryptedFrameSize(n int) int { return n + 1 }
func (f *fakeDave) Encrypt(_ uint32, frame, out []byte) (int, error) {
	return copy(out, append([]byte{'E'}, frame...)), nil
}
func (f *fakeDave) AddUser(id godave.UserID)    { f.calls <- "add " + string(id) }
func (f *fakeDave) RemoveUser(id godave.UserID) { f.calls <- "remove " + string(id) }
func (f *fakeDave) OnSelectProtocolAck(v uint16) {
	f.calls <- fmt.Sprint("ack ", v)
	f.cb.SendMLSKeyPackage([]byte{0xAA})
}
func (f *fakeDave) OnDavePrepareTransition(id, v uint16) {
	f.calls <- fmt.Sprint("prepare transition ", id, " ", v)
	f.cb.SendReadyForTransition(id)
}
func (f *fakeDave) OnDaveExecuteTransition(id uint16) { f.calls <- fmt.Sprint("execute ", id) }
func (f *fakeDave) OnDaveMLSExternalSenderPackage(b []byte) {
	f.calls <- fmt.Sprintf("external %x", b)
}
func (f *fakeDave) OnDaveMLSPrepareCommitTransition(id uint16, b []byte) {
	f.calls <- fmt.Sprintf("commit %d %x", id, b)
}
func (f *fakeDave) OnDaveMLSWelcome(id uint16, b []byte) {
	f.calls <- fmt.Sprintf("welcome %d %x", id, b)
}

type fakeServer struct {
	t     *testing.T
	ws    *websocket.Conn
	udp   *net.UDPConn
	pkts  chan []byte
	dave  *fakeDave
	conns chan *websocket.Conn
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{t: t, pkts: make(chan []byte, 16), conns: make(chan *websocket.Conn, 1),
		dave: &fakeDave{calls: make(chan string, 32)}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "8" {
			t.Errorf("gateway version = %q, want 8", r.URL.Query().Get("v"))
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		f.conns <- ws
	}))
	t.Cleanup(srv.Close)
	old := gatewayURL
	gatewayURL = func(string) string { return "ws" + strings.TrimPrefix(srv.URL, "http") + "/?v=8" }
	t.Cleanup(func() { gatewayURL = old })

	var err error
	if f.udp, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.udp.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := f.udp.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n == 74 && binary.BigEndian.Uint16(buf) == 1 {
				resp := make([]byte, 74)
				binary.BigEndian.PutUint16(resp, 2)
				binary.BigEndian.PutUint16(resp[2:], 70)
				copy(resp[4:8], buf[4:8])
				copy(resp[8:], "198.51.100.7")
				binary.BigEndian.PutUint16(resp[72:], 40000)
				f.udp.WriteToUDP(resp, addr)
				continue
			}
			f.pkts <- bytes.Clone(buf[:n])
		}
	}()
	return f
}

func (f *fakeServer) send(op int, seq int, d any) {
	f.t.Helper()
	if err := f.ws.WriteJSON(map[string]any{"op": op, "seq": seq, "d": d}); err != nil {
		f.t.Fatal(err)
	}
}

// next reads client messages, skipping heartbeats unless op is opHeartbeat.
func (f *fakeServer) next(op int) (kind int, m map[string]any, raw []byte) {
	f.t.Helper()
	for {
		f.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		kind, raw, err := f.ws.ReadMessage()
		if err != nil {
			f.t.Fatalf("waiting for op %d: %v", op, err)
		}
		if kind == websocket.BinaryMessage {
			return kind, nil, raw
		}
		var msg struct {
			Op int            `json:"op"`
			D  map[string]any `json:"d"`
		}
		json.Unmarshal(raw, &msg)
		if msg.Op == op {
			return kind, msg.D, raw
		}
		if msg.Op != opHeartbeat {
			f.t.Fatalf("got op %d, want %d", msg.Op, op)
		}
	}
}

func (f *fakeServer) wantCall(want string) {
	f.t.Helper()
	select {
	case got := <-f.dave.calls:
		if got != want {
			f.t.Fatalf("dave call = %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		f.t.Fatalf("no dave call, want %q", want)
	}
}

var testKey = bytes.Repeat([]byte{3}, 32)

// dial runs the handshake against the fake server with fakeDave.
func (f *fakeServer) dial() *Conn { return f.dialWith(true, 1) }

// dialWith runs the handshake; fake picks fakeDave over the real dave-go
// session, version is the dave_protocol_version the server selects.
func (f *fakeServer) dialWith(fake bool, version int) *Conn {
	f.t.Helper()
	type result struct {
		c   *Conn
		err error
	}
	res := make(chan result, 1)
	go func() {
		p := Params{
			GuildID: "10", ChannelID: "20", UserID: "30", SessionID: "sess", Token: "tok", Endpoint: "x",
			Logger: slog.New(slog.DiscardHandler),
		}
		if fake {
			p.Dave = func(_ *slog.Logger, _ godave.UserID, cb godave.Callbacks) godave.Session {
				f.dave.cb = cb
				return f.dave
			}
		}
		c, err := Dial(context.Background(), p)
		res <- result{c, err}
	}()

	f.ws = <-f.conns
	f.send(opHello, 0, map[string]any{"heartbeat_interval": 20.0})
	_, id, _ := f.next(opIdentify)
	if id["server_id"] != "10" || id["session_id"] != "sess" || id["token"] != "tok" || id["max_dave_protocol_version"] != 1.0 {
		f.t.Fatalf("identify = %v", id)
	}
	if fake {
		f.wantCall("channel 20")
	}

	port := f.udp.LocalAddr().(*net.UDPAddr).Port
	f.send(opReady, 1, map[string]any{"ssrc": 42, "ip": "127.0.0.1", "port": port,
		"modes": []string{ModeXChaCha20, ModeAES256GCM, "xsalsa20_poly1305"}})

	_, sel, _ := f.next(opSelectProtocol)
	data := sel["data"].(map[string]any)
	if sel["protocol"] != "udp" || data["address"] != "198.51.100.7" || data["port"] != 40000.0 || data["mode"] != ModeAES256GCM {
		f.t.Fatalf("select protocol = %v", sel)
	}

	key := make([]int, len(testKey))
	for i, b := range testKey {
		key[i] = int(b)
	}
	f.send(opSessionDescription, 2, map[string]any{"mode": ModeAES256GCM, "secret_key": key, "dave_protocol_version": version})
	if fake {
		f.wantCall("ssrc 42")
		f.wantCall("ack 1")
		if kind, _, raw := f.next(-1); kind != websocket.BinaryMessage || !bytes.Equal(raw, []byte{opDaveKeyPackage, 0xAA}) {
			f.t.Fatalf("key package frame = %x", raw)
		}
	}

	r := <-res
	if r.err != nil {
		f.t.Fatal(r.err)
	}
	f.t.Cleanup(func() { r.c.Close() })
	return r.c
}

func TestConnHandshakeAndSend(t *testing.T) {
	f := newFakeServer(t)
	c := f.dial()

	if err := c.Speaking(true); err != nil {
		t.Fatal(err)
	}
	if _, sp, _ := f.next(opSpeaking); sp["speaking"] != 1.0 || sp["ssrc"] != 42.0 {
		t.Fatalf("speaking = %v", sp)
	}

	open, _ := newSealer(ModeAES256GCM, testKey)
	for i := range 2 {
		if err := c.WriteOpus([]byte("opus")); err != nil {
			t.Fatal(err)
		}
		pkt := <-f.pkts
		if seq := binary.BigEndian.Uint16(pkt[2:]); seq != uint16(i) {
			t.Fatalf("seq = %d, want %d", seq, i)
		}
		if ts := binary.BigEndian.Uint32(pkt[4:]); ts != uint32(i*FrameSamples) {
			t.Fatalf("timestamp = %d", ts)
		}
		if ssrc := binary.BigEndian.Uint32(pkt[8:]); ssrc != 42 {
			t.Fatalf("ssrc = %d", ssrc)
		}
		got, err := open.open(pkt)
		if err != nil || string(got) != "Eopus" { // 'E' marks the fake DAVE encrypt
			t.Fatalf("payload = %q, %v", got, err)
		}
	}
}

func TestConnDaveDispatch(t *testing.T) {
	f := newFakeServer(t)
	f.dial()

	f.send(opClientsConnect, 3, map[string]any{"user_ids": []string{"1", "2"}})
	f.wantCall("add 1")
	f.wantCall("add 2")
	f.send(opClientDisconnect, 4, map[string]any{"user_id": "1"})
	f.wantCall("remove 1")

	f.ws.WriteMessage(websocket.BinaryMessage, []byte{0, 5, opDaveExternalSender, 1, 2, 3})
	f.wantCall("external 010203")
	f.ws.WriteMessage(websocket.BinaryMessage, []byte{0, 6, opDaveAnnounceCommit, 0, 7, 9, 9})
	f.wantCall("commit 7 0909")
	f.ws.WriteMessage(websocket.BinaryMessage, []byte{0, 7, opDaveWelcome, 0, 8, 0xCC})
	f.wantCall("welcome 8 cc")

	f.send(opDavePrepareTransition, 8, map[string]any{"transition_id": 11, "protocol_version": 1})
	f.wantCall("prepare transition 11 1")
	if _, rd, _ := f.next(opDaveTransitionReady); rd["transition_id"] != 11.0 {
		t.Fatalf("transition ready = %v", rd)
	}
	f.send(opDaveExecuteTransition, 9, map[string]any{"transition_id": 11})
	f.wantCall("execute 11")

	// The heartbeat acks the last sequence number seen, JSON or binary.
	for {
		_, hb, _ := f.next(opHeartbeat)
		if hb["seq_ack"] == 9.0 {
			break
		}
	}
}

func TestConnServerClose(t *testing.T) {
	f := newFakeServer(t)
	c := f.dial()
	f.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4017, "E2EE/DAVE protocol required"))
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("not closed")
	}
	if ce, ok := errors.AsType[*CloseError](c.Err()); !ok || ce.Code != 4017 {
		t.Fatalf("err = %v", c.Err())
	}
	if err := c.WriteOpus([]byte("x")); err == nil {
		t.Fatal("write after close succeeded")
	}
}

// The gateway closing right after Ready must still close the UDP socket,
// which would otherwise block IP discovery against a silent server forever.
func TestConnGatewayClosesDuringHandshake(t *testing.T) {
	for range 20 {
		f := newFakeServer(t)
		silent, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { silent.Close() })

		res := make(chan error, 1)
		go func() {
			_, err := Dial(context.Background(), Params{GuildID: "10", ChannelID: "20", UserID: "30",
				Endpoint: "x", Logger: slog.New(slog.DiscardHandler),
				Dave: func(_ *slog.Logger, _ godave.UserID, cb godave.Callbacks) godave.Session {
					f.dave.cb = cb
					return f.dave
				}})
			res <- err
		}()

		f.ws = <-f.conns
		f.next(opIdentify)
		f.send(opReady, 1, map[string]any{"ssrc": 42, "ip": "127.0.0.1",
			"port": silent.LocalAddr().(*net.UDPAddr).Port, "modes": []string{ModeXChaCha20}})
		f.ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(4006, "session no longer valid"))

		select {
		case err := <-res:
			if err == nil {
				t.Fatal("Dial succeeded after the gateway closed")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Dial hung after the gateway closed")
		}
	}
}

func TestConnRealDave(t *testing.T) {
	t.Run("no E2EE", func(t *testing.T) {
		f := newFakeServer(t)
		c := f.dialWith(false, 0)
		if err := c.WriteOpus([]byte("opus")); err != nil {
			t.Fatalf("transport-only channel held frames: %v", err)
		}
		open, _ := newSealer(ModeAES256GCM, testKey)
		if got, err := open.open(<-f.pkts); err != nil || string(got) != "opus" {
			t.Fatalf("payload = %q, %v", got, err)
		}
	})
	t.Run("DAVE v1", func(t *testing.T) {
		f := newFakeServer(t)
		c := f.dialWith(false, 1)
		kind, _, raw := f.next(-1)
		if kind != websocket.BinaryMessage || raw[0] != opDaveKeyPackage || len(raw) < 64 {
			t.Fatalf("no MLS key package sent: %x", raw)
		}
		if err := c.WriteOpus([]byte("opus")); !errors.Is(err, ErrNotReady) {
			t.Fatalf("write before the MLS group exists = %v, want ErrNotReady", err)
		}
	})
}
