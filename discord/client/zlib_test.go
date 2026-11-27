package client

import (
	"bytes"
	"compress/zlib"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/discord"
)

// zlibStream compresses messages as Discord does: one zlib stream, each
// message ending on a sync flush.
type zlibStream struct {
	buf bytes.Buffer
	w   *zlib.Writer
}

func newZlibStream() *zlibStream {
	z := &zlibStream{}
	z.w = zlib.NewWriter(&z.buf)
	return z
}

func (z *zlibStream) message(t *testing.T, payload string) []byte {
	t.Helper()
	z.buf.Reset()
	if _, err := z.w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := z.w.Flush(); err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(z.buf.Bytes())
}

func TestInflaterFollowsTheStreamAcrossMessages(t *testing.T) {
	stream, z := newZlibStream(), &inflater{}

	// Repeats refer back into earlier messages, and big ones past the window.
	big := strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 2000) // 72 KB
	for i, want := range []string{`{"op":10}`, big, big, `{"op":11}`, big[:500] + "x" + big[:500], big} {
		got, err := z.inflate(stream.message(t, want))
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if string(got) != want {
			t.Fatalf("message %d: got %d bytes, want %d", i, len(got), len(want))
		}
	}
	if len(z.window) != deflateWindow {
		t.Errorf("window = %d bytes, want %d", len(z.window), deflateWindow)
	}
}

func TestInflaterJoinsASplitMessage(t *testing.T) {
	stream, z := newZlibStream(), &inflater{}
	msg := stream.message(t, `{"op":10,"d":{"heartbeat_interval":41250}}`)

	if got, err := z.inflate(msg[:len(msg)/2]); got != nil || err != nil {
		t.Fatalf("first half = %q, %v; want nothing yet", got, err)
	}
	got, err := z.inflate(msg[len(msg)/2:])
	if err != nil || string(got) != `{"op":10,"d":{"heartbeat_interval":41250}}` {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestInflaterRejectsAnInvalidStream(t *testing.T) {
	if _, err := (&inflater{}).inflate([]byte{0x01, 0x02, 0x00, 0x00, 0xff, 0xff}); err == nil {
		t.Error("an invalid zlib header was accepted")
	}

	// A stream that was closed ends rather than going on after a sync flush.
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write([]byte("{}"))
	w.Flush()
	w.Close()
	if _, err := (&inflater{}).inflate(append(buf.Bytes(), zlibSuffix...)); err == nil {
		t.Error("an ended stream was accepted")
	}
}

func TestGatewayIsCompressed(t *testing.T) {
	big := strings.Repeat("x", 100<<10) // past the window, and in two websocket messages
	got := make(chan string, 2)

	s, g := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent(big), 2, "MESSAGE_CREATE")

		// Discord may split a payload; the client waits for the rest.
		c.buf.Reset()
		c.z.Write([]byte(`{"op":0,"s":3,"t":"MESSAGE_CREATE","d":{"id":"split","channel_id":"c1","content":"split","author":{"id":"7"}}}`))
		c.z.Flush()
		data := bytes.Clone(c.buf.Bytes())
		for _, part := range [][]byte{data[:len(data)/2], data[len(data)/2:]} {
			if err := c.ws.WriteMessage(websocket.BinaryMessage, part); err != nil {
				t.Error(err)
			}
		}
		c.waitClosed()
	})
	s.OnMessageCreate(func(_ *Session, m *discord.MessageCreate) { got <- m.Content })

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	contents := map[string]bool{}
	for range 2 {
		select {
		case c := <-got:
			contents[c] = true
		case <-t.Context().Done():
			t.Fatal("events not received")
		}
	}
	if !contents[big] || !contents["split"] {
		t.Errorf("did not get both events intact")
	}
	if g.compressed.Load() != 1 {
		t.Errorf("compressed connections = %d, want 1", g.compressed.Load())
	}
}

func TestGatewayCompressionCanBeDisabled(t *testing.T) {
	received := make(chan struct{})

	s, g := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent("plain"), 2, "MESSAGE_CREATE")
		c.waitClosed()
	})
	s.DisableCompression = true
	s.OnMessageCreate(func(*Session, *discord.MessageCreate) { close(received) })

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, received, "the event")
	if g.compressed.Load() != 0 {
		t.Error("compression was asked for")
	}
}
