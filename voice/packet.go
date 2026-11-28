package voice

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"golang.org/x/crypto/chacha20poly1305"
)

// Transport encryption modes Discord still accepts. xchacha is required,
// AES-GCM is preferred when the server offers it.
const (
	ModeAES256GCM = "aead_aes256_gcm_rtpsize"
	ModeXChaCha20 = "aead_xchacha20_poly1305_rtpsize"
)

const (
	rtpHeaderSize = 12
	// FrameSamples is the RTP timestamp step for one 20ms Opus frame at 48kHz.
	FrameSamples = 960
)

// SilenceFrame is an Opus frame of silence. Send a few after the last real
// frame so receivers don't interpolate.
var SilenceFrame = []byte{0xF8, 0xFF, 0xFE}

// rtpHeader writes a 12-byte RTP header (version 2, payload type 0x78) into b.
func rtpHeader(b []byte, seq uint16, timestamp, ssrc uint32) {
	b[0], b[1] = 0x80, 0x78
	binary.BigEndian.PutUint16(b[2:], seq)
	binary.BigEndian.PutUint32(b[4:], timestamp)
	binary.BigEndian.PutUint32(b[8:], ssrc)
}

// sealer encrypts RTP payloads for one of the *_rtpsize modes: the RTP header
// is the additional data, and a 4-byte big-endian counter is the start of the
// nonce (zero-padded) and is appended to the packet.
type sealer struct {
	aead  cipher.AEAD
	nonce uint32
}

func newSealer(mode string, key []byte) (*sealer, error) {
	var aead cipher.AEAD
	var err error
	switch mode {
	case ModeAES256GCM:
		var block cipher.Block
		if block, err = aes.NewCipher(key); err == nil {
			aead, err = cipher.NewGCM(block)
		}
	case ModeXChaCha20:
		aead, err = chacha20poly1305.NewX(key)
	default:
		return nil, fmt.Errorf("voice: unsupported encryption mode %q", mode)
	}
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal appends header || ciphertext+tag || nonce counter to dst.
func (s *sealer) seal(dst, header, payload []byte) []byte {
	s.nonce++
	nonce := make([]byte, s.aead.NonceSize())
	binary.BigEndian.PutUint32(nonce, s.nonce)
	dst = append(dst, header...)
	dst = s.aead.Seal(dst, nonce, payload, header)
	return binary.BigEndian.AppendUint32(dst, s.nonce)
}

// open reverses seal for a packet with a plain 12-byte header.
func (s *sealer) open(packet []byte) ([]byte, error) {
	if len(packet) < rtpHeaderSize+s.aead.Overhead()+4 {
		return nil, errors.New("voice: packet too short")
	}
	nonce := make([]byte, s.aead.NonceSize())
	copy(nonce, packet[len(packet)-4:])
	header := packet[:rtpHeaderSize]
	return s.aead.Open(nil, nonce, packet[rtpHeaderSize:len(packet)-4], header)
}

// discoverIP runs Discord's IP discovery over conn and returns the public
// address and port the voice server sees.
func discoverIP(conn net.Conn, ssrc uint32) (string, uint16, error) {
	req := make([]byte, 74)
	binary.BigEndian.PutUint16(req[0:], 1)
	binary.BigEndian.PutUint16(req[2:], 70)
	binary.BigEndian.PutUint32(req[4:], ssrc)
	if _, err := conn.Write(req); err != nil {
		return "", 0, err
	}
	resp := make([]byte, 74)
	n, err := conn.Read(resp)
	if err != nil {
		return "", 0, err
	}
	return parseDiscovery(resp[:n])
}

func parseDiscovery(b []byte) (string, uint16, error) {
	if len(b) < 74 || binary.BigEndian.Uint16(b) != 2 {
		return "", 0, errors.New("voice: bad IP discovery response")
	}
	addr := b[8:72]
	if i := bytes.IndexByte(addr, 0); i >= 0 {
		addr = addr[:i]
	}
	return string(addr), binary.BigEndian.Uint16(b[72:]), nil
}
