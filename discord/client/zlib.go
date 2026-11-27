package client

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
)

// zlibSuffix ends each message of a zlib-stream connection: the sync flush
// after which everything sent so far can be inflated.
var zlibSuffix = []byte{0x00, 0x00, 0xff, 0xff}

// deflateWindow is how far back a deflate stream refers to earlier output.
const deflateWindow = 32 << 10

// inflater decompresses the messages of a zlib-stream gateway connection,
// which are parts of one deflate stream. A message ends on a sync flush, so
// the next starts a new deflate block that refers back only to the output
// before it; a reader started afresh on it, with the last 32 KiB of output as
// its dictionary, inflates it as the whole stream would.
type inflater struct {
	pending []byte // messages not yet ended by zlibSuffix
	started bool   // the zlib header was read
	r       io.ReadCloser
	window  []byte // the last deflateWindow bytes inflated
}

// inflate takes a binary message and returns the payload it completes, or nil
// if Discord split the payload and more of it is to come. After an error the
// stream cannot be continued, so the connection must be dropped.
func (z *inflater) inflate(msg []byte) ([]byte, error) {
	data := msg // most payloads come whole, and need no copy
	if len(z.pending) > 0 || !bytes.HasSuffix(msg, zlibSuffix) {
		z.pending = append(z.pending, msg...)
		if !bytes.HasSuffix(z.pending, zlibSuffix) {
			return nil, nil
		}
		data, z.pending = z.pending, nil
	}

	if !z.started {
		// CMF and FLG: deflate with a window of at most 32 KiB, and a valid check.
		if len(data) < 2 || data[0]&0x0f != 8 || data[0]>>4 > 7 || (uint(data[0])<<8|uint(data[1]))%31 != 0 || data[1]&0x20 != 0 {
			return nil, errors.New("discord: invalid zlib header")
		}
		data = data[2:]
		z.started = true
	}

	src := bytes.NewReader(data)
	if z.r == nil {
		z.r = flate.NewReaderDict(src, z.window)
	} else if err := z.r.(flate.Resetter).Reset(src, z.window); err != nil {
		return nil, err
	}

	out, err := io.ReadAll(z.r)
	// The reader stops where the message does, after the sync flush, wanting
	// the next block: the stream itself never ends.
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		if err == nil {
			err = errors.New("discord: zlib stream ended")
		}
		return nil, err
	}

	// Only the tail of a large payload is kept, so that the window's array
	// never grows past twice its size.
	if len(out) >= deflateWindow {
		z.window = append(z.window[:0], out[len(out)-deflateWindow:]...)
	} else {
		z.window = append(z.window, out...)
		if extra := len(z.window) - deflateWindow; extra > 0 {
			z.window = append(z.window[:0], z.window[extra:]...)
		}
	}
	return out, nil
}
