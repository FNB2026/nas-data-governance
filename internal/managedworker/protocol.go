package managedworker

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Request is a decoded frame. It carries no path, no command and no
// execution hint: only the named object identity and its fixed parameters.
type Request struct {
	Op     string
	Fields map[string]json.RawMessage
}

// DecodeRequest accepts a frame only when it is a bounded JSON object with a
// known op and no key outside that op's fixed set. Unknown keys are refused
// rather than ignored, so a caller cannot extend the protocol from outside.
func DecodeRequest(raw []byte) (Request, error) {
	if len(raw) == 0 || len(raw) > maxRequestBytes {
		return Request{}, ErrProtocol
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return Request{}, ErrProtocol
	}
	rawOp, ok := fields["op"]
	if !ok {
		return Request{}, ErrProtocol
	}
	var op string
	if json.Unmarshal(rawOp, &op) != nil {
		return Request{}, ErrProtocol
	}
	allowed, ok := allowedKeys[op]
	if !ok {
		return Request{}, ErrProtocol
	}
	delete(fields, "op")
	for key := range fields {
		found := false
		for _, candidate := range allowed {
			if candidate == key {
				found = true
				break
			}
		}
		if !found {
			return Request{}, ErrProtocol
		}
	}
	for key, value := range fields {
		var text string
		if json.Unmarshal(value, &text) == nil && pathLike(text) {
			return Request{}, ErrProtocol
		}
		_ = key
	}
	return Request{Op: op, Fields: fields}, nil
}

// String returns a required string field.
func (r Request) String(key string) (string, error) {
	raw, ok := r.Fields[key]
	if !ok {
		return "", ErrProtocol
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || pathLike(value) {
		return "", ErrProtocol
	}
	return value, nil
}

// Int64 returns a required integer field. Booleans and strings are refused so
// that a numeric bound cannot be widened by changing a JSON type.
func (r Request) Int64(key string) (int64, error) {
	raw, ok := r.Fields[key]
	if !ok {
		return 0, ErrProtocol
	}
	var value int64
	if json.Unmarshal(raw, &value) != nil || strconv.FormatInt(value, 10) != strings.TrimSpace(string(raw)) {
		return 0, ErrProtocol
	}
	return value, nil
}

// OpaqueID returns a required 64-character lowercase hexadecimal identifier.
func (r Request) OpaqueID(key string) (string, error) {
	value, err := r.String(key)
	if err != nil || !opaqueID(value) {
		return "", ErrProtocol
	}
	return value, nil
}
