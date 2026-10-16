package managedworker

import (
	"encoding/binary"
	"io"
)

// ReadRequest reads one length-prefixed bounded frame and decodes it. A frame
// that is empty or over the limit is refused before any allocation.
func (c *Conn) ReadRequest() (Request, error) {
	var header [4]byte
	if _, err := io.ReadFull(c.conn, header[:]); err != nil {
		return Request{}, ErrProtocol
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxRequestBytes {
		return Request{}, ErrProtocol
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(c.conn, raw); err != nil {
		return Request{}, ErrProtocol
	}
	return DecodeRequest(raw)
}

// WriteResponse writes one bounded frame. A response that would exceed the
// frame limit is refused rather than truncated.
func (c *Conn) WriteResponse(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxRequestBytes {
		return ErrProtocol
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(raw)))
	if _, err := c.conn.Write(header[:]); err != nil {
		return err
	}
	_, err := c.conn.Write(raw)
	return err
}

// frame encodes a request body for a client. It is exported so that a test
// client cannot accidentally bypass the size bound.
func frame(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > maxRequestBytes {
		return nil, ErrProtocol
	}
	out := make([]byte, 4+len(raw))
	binary.BigEndian.PutUint32(out, uint32(len(raw)))
	copy(out[4:], raw)
	return out, nil
}
