package node

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// oggReader splits an Ogg/Opus stream into Opus packets, skipping the
// OpusHead and OpusTags header packets. It does not check page CRCs: the
// input is our own ffmpeg's stdout.
type oggReader struct {
	r       *bufio.Reader
	lacing  []byte
	body    []byte
	partial []byte
}

func newOggReader(r io.Reader) *oggReader { return &oggReader{r: bufio.NewReader(r)} }

// Packet returns the next Opus packet, or io.EOF at the end of the stream.
func (o *oggReader) Packet() ([]byte, error) {
	for {
		for len(o.lacing) > 0 {
			n := int(o.lacing[0])
			o.lacing = o.lacing[1:]
			o.partial = append(o.partial, o.body[:n]...)
			o.body = o.body[n:]
			if n == 255 {
				continue // packet continues in the next segment
			}
			pkt := o.partial
			o.partial = nil
			if bytes.HasPrefix(pkt, []byte("OpusHead")) || bytes.HasPrefix(pkt, []byte("OpusTags")) {
				continue
			}
			return pkt, nil
		}
		if err := o.readPage(); err != nil {
			return nil, err
		}
	}
}

func (o *oggReader) readPage() error {
	var h [27]byte
	if _, err := io.ReadFull(o.r, h[:]); err != nil {
		return err
	}
	if string(h[:4]) != "OggS" {
		return errors.New("ogg: bad page signature")
	}
	o.lacing = make([]byte, h[26])
	if _, err := io.ReadFull(o.r, o.lacing); err != nil {
		return noEOF(err)
	}
	size := 0
	for _, n := range o.lacing {
		size += int(n)
	}
	o.body = make([]byte, size)
	_, err := io.ReadFull(o.r, o.body)
	return noEOF(err)
}

// noEOF turns an EOF inside a page into ErrUnexpectedEOF.
func noEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}
