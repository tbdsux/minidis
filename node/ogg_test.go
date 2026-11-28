package node

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// oggPage builds a page from raw lacing values and body (CRC left zero).
func oggPage(lacing []byte, body []byte) []byte {
	h := make([]byte, 27)
	copy(h, "OggS")
	h[26] = byte(len(lacing))
	return append(append(h, lacing...), body...)
}

func TestOggReader(t *testing.T) {
	head := append([]byte("OpusHead"), make([]byte, 11)...)
	tags := []byte("OpusTags\x00\x00\x00\x00")
	a := bytes.Repeat([]byte{'a'}, 300) // two segments: 255 + 45
	b := bytes.Repeat([]byte{'b'}, 265) // spans pages: 255 | 10

	var stream []byte
	stream = append(stream, oggPage([]byte{19}, head)...)
	stream = append(stream, oggPage([]byte{byte(len(tags))}, tags)...)
	stream = append(stream, oggPage([]byte{255, 45, 255}, append(a, b[:255]...))...)
	stream = append(stream, oggPage([]byte{10, 3}, append(b[255:], 'c', 'c', 'c'))...)

	r := newOggReader(bytes.NewReader(stream))
	for _, want := range [][]byte{a, b, []byte("ccc")} {
		got, err := r.Packet()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("packet = %q (%d bytes), %v; want %d bytes", got[:min(len(got), 8)], len(got), err, len(want))
		}
	}
	if _, err := r.Packet(); err != io.EOF {
		t.Fatalf("end = %v, want EOF", err)
	}
}

func TestOggReaderErrors(t *testing.T) {
	page := oggPage([]byte{5}, []byte("hello"))
	if _, err := newOggReader(bytes.NewReader(page[:len(page)-2])).Packet(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated page = %v", err)
	}
	bad := bytes.Clone(page)
	bad[0] = 'X'
	if _, err := newOggReader(bytes.NewReader(bad)).Packet(); err == nil {
		t.Fatal("bad signature accepted")
	}
}
