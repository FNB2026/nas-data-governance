package managedworker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodeRequestAcceptsFixedOps(t *testing.T) {
	id := strings.Repeat("a", 64)
	auth := strings.Repeat("b", 64)
	for _, raw := range []string{
		`{"op":"PING"}`,
		`{"op":"STATUS"}`,
		`{"op":"SEAL","object_id":"` + id + `","size":16,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"PURGE","object_id":"` + id + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"INSPECT","object_id":"` + id + `","size":4,"sha256":"` + auth + `","device":2049,"inode":77}`,
	} {
		request, err := DecodeRequest([]byte(raw))
		if err != nil {
			t.Fatal("fixed op refused", raw, err)
		}
		if request.Op == "" {
			t.Fatal("op missing", raw)
		}
	}
}

// This is the guarantee that a caller cannot smuggle a location, a command or
// an execution hint into an authorized request.
func TestDecodeRequestRefusesUnknownKeys(t *testing.T) {
	for _, raw := range []string{
		`{"op":"PING","source":"/etc/passwd"}`,
		`{"op":"SEAL","path":"/var/lib/ndg"}`,
		`{"op":"PURGE","root":"/"}`,
		`{"op":"STATUS","exec":"rm"}`,
		`{"op":"SEAL","object_id":"` + strings.Repeat("a", 64) + `","..":1}`,
	} {
		if _, err := DecodeRequest([]byte(raw)); !errors.Is(err, ErrProtocol) {
			t.Fatal("unknown key accepted", raw, err)
		}
	}
}

// An allowed key with a location-shaped value must still be refused. Without
// this case the path guard is only reachable through the unknown-key check,
// which would leave it untested.
func TestDecodeRequestRefusesPathLikeValueOnAllowedKey(t *testing.T) {
	auth := strings.Repeat("b", 64)
	id := strings.Repeat("a", 64)
	for _, raw := range []string{
		`{"op":"SEAL","object_id":"/etc/passwd","size":16,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"SEAL","object_id":"../escape","size":16,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"PURGE","object_id":"` + id + `","authorization_sha256":"var/lib/ndg"}`,
	} {
		if _, err := DecodeRequest([]byte(raw)); !errors.Is(err, ErrProtocol) {
			t.Fatal("path-like value accepted on an allowed key", raw, err)
		}
	}
}

func TestDecodeRequestRefusesMalformedFrames(t *testing.T) {
	id := strings.Repeat("a", 64)
	oversized := `{"op":"PING","pad":"` + strings.Repeat("x", maxRequestBytes) + `"}`
	for _, raw := range []string{
		``,
		`[]`,
		`"PING"`,
		`null`,
		`{}`,
		`{"op":42}`,
		`{"op":"EXEC"}`,
		`{"op":"ping"}`,
		`{"op":"PING","object_id":"/etc"}`,
		notJSON,
		oversized,
	} {
		if _, err := DecodeRequest([]byte(raw)); !errors.Is(err, ErrProtocol) {
			t.Fatal("malformed frame accepted", raw[:min(len(raw), 48)], err)
		}
	}
	_ = id
}

const notJSON = `{"op":"PING"`

func TestRequestFieldTypesAreStrict(t *testing.T) {
	auth := strings.Repeat("b", 64)
	raw := []byte(`{"op":"SEAL","object_id":"` + strings.Repeat("a", 64) + `","size":16,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`)
	request, err := DecodeRequest(raw)
	if err != nil {
		t.Fatal("valid frame refused", err)
	}
	if size, err := request.Int64("size"); err != nil || size != 16 {
		t.Fatal("size mismatch", size, err)
	}
	if _, err := request.Int64("sha256"); !errors.Is(err, ErrProtocol) {
		t.Fatal("non-integer accepted as Int64")
	}
	if _, err := request.Int64("missing"); !errors.Is(err, ErrProtocol) {
		t.Fatal("missing Int64 accepted")
	}
	if _, err := request.String("missing"); !errors.Is(err, ErrProtocol) {
		t.Fatal("missing String accepted")
	}
	if _, err := request.OpaqueID("object_id"); err != nil {
		t.Fatal("valid opaque id refused", err)
	}
	if _, err := request.OpaqueID("sha256"); err != nil {
		t.Fatal("valid opaque sha refused", err)
	}
	if _, err := request.OpaqueID("size"); !errors.Is(err, ErrProtocol) {
		t.Fatal("non-id accepted as OpaqueID")
	}
}

// A numeric bound must not be widened by changing the JSON type.
func TestRequestRefusesNumericTypeWidening(t *testing.T) {
	auth := strings.Repeat("b", 64)
	for _, raw := range []string{
		`{"op":"SEAL","object_id":"` + strings.Repeat("a", 64) + `","size":"16","sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"SEAL","object_id":"` + strings.Repeat("a", 64) + `","size":true,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"SEAL","object_id":"` + strings.Repeat("a", 64) + `","size":16.0,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
	} {
		request, err := DecodeRequest([]byte(raw))
		if err != nil {
			continue
		}
		if _, err := request.Int64("size"); !errors.Is(err, ErrProtocol) {
			t.Fatal("widened numeric accepted", raw, err)
		}
	}
}

func TestFrameBounds(t *testing.T) {
	if _, err := frame(nil); !errors.Is(err, ErrProtocol) {
		t.Fatal("empty frame accepted")
	}
	if _, err := frame(make([]byte, maxRequestBytes+1)); !errors.Is(err, ErrProtocol) {
		t.Fatal("oversized frame accepted")
	}
	out, err := frame([]byte(`{"op":"PING"}`))
	if err != nil {
		t.Fatal("valid frame refused", err)
	}
	if len(out) != 4+len(`{"op":"PING"}`) {
		t.Fatal("frame length wrong", len(out))
	}
}

func TestOpsAreAllReachable(t *testing.T) {
	// A listed op must be decodable, otherwise the allow-list advertises an
	// operation the protocol cannot actually accept.
	for _, op := range Ops() {
		raw, err := json.Marshal(map[string]string{"op": op})
		if err != nil {
			t.Fatal("marshal failed", err)
		}
		if _, err := DecodeRequest(raw); err != nil {
			t.Fatal("advertised op not decodable", op, err)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
