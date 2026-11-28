package voice

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRTPHeader(t *testing.T) {
	b := make([]byte, rtpHeaderSize)
	rtpHeader(b, 0x0102, 0x03040506, 0x0708090a)
	want := []byte{0x80, 0x78, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if !bytes.Equal(b, want) {
		t.Fatalf("header = %x, want %x", b, want)
	}
}

func TestSealerRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	for _, mode := range []string{ModeAES256GCM, ModeXChaCha20} {
		t.Run(mode, func(t *testing.T) {
			s, err := newSealer(mode, key)
			if err != nil {
				t.Fatal(err)
			}
			header := make([]byte, rtpHeaderSize)
			rtpHeader(header, 1, 960, 42)
			payload := []byte("opus frame")

			p1 := s.seal(nil, header, payload)
			p2 := s.seal(nil, header, payload)

			if !bytes.Equal(p1[:rtpHeaderSize], header) {
				t.Fatal("header not left in the clear")
			}
			if len(p1) != rtpHeaderSize+len(payload)+s.aead.Overhead()+4 {
				t.Fatalf("len = %d", len(p1))
			}
			if n := binary.BigEndian.Uint32(p1[len(p1)-4:]); n != 1 {
				t.Fatalf("first nonce = %d, want 1", n)
			}
			if bytes.Equal(p1, p2) {
				t.Fatal("nonce did not advance")
			}

			r, _ := newSealer(mode, key)
			got, err := r.open(p2)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("open = %q, %v", got, err)
			}
			p2[3] ^= 1 // header is AAD: tampering must fail
			if _, err := r.open(p2); err == nil {
				t.Fatal("tampered header opened")
			}
		})
	}
}

func TestNewSealerRejectsOldModes(t *testing.T) {
	if _, err := newSealer("xsalsa20_poly1305", make([]byte, 32)); err == nil {
		t.Fatal("deprecated mode accepted")
	}
}

func TestParseDiscovery(t *testing.T) {
	b := make([]byte, 74)
	binary.BigEndian.PutUint16(b, 2)
	binary.BigEndian.PutUint16(b[2:], 70)
	copy(b[8:], "203.0.113.9")
	binary.BigEndian.PutUint16(b[72:], 50001)
	ip, port, err := parseDiscovery(b)
	if err != nil || ip != "203.0.113.9" || port != 50001 {
		t.Fatalf("got %q %d %v", ip, port, err)
	}
	if _, _, err := parseDiscovery(b[:20]); err == nil {
		t.Fatal("short response accepted")
	}
}
