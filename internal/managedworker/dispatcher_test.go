package managedworker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDispatchRefusesObjectOperationsWithoutAuthorization(t *testing.T) {
	id := strings.Repeat("a", 64)
	auth := strings.Repeat("b", 64)
	// Complete, well-formed payloads: the refusal must come from the
	// authorization gate, not from a missing or malformed field.
	for _, raw := range []string{
		`{"op":"SEAL","object_id":"` + id + `","size":16,"sha256":"` + auth + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"PURGE","object_id":"` + id + `","authorization_sha256":"` + auth + `"}`,
		`{"op":"INSPECT","object_id":"` + id + `","size":4,"sha256":"` + auth + `","device":2049,"inode":77}`,
	} {
		request, err := DecodeRequest([]byte(raw))
		if err != nil {
			t.Fatal("valid payload refused by the protocol", raw, err)
		}
		if _, err := Dispatch(context.Background(), request); !errors.Is(err, ErrAuthorization) {
			t.Fatal("object operation was executable without business authorization", request.Op, err)
		}
	}
}

func TestDispatchExecutableOpsStayMinimal(t *testing.T) {
	for _, op := range Executable() {
		request, err := DecodeRequest([]byte(`{"op":"` + op + `"}`))
		if err != nil {
			t.Fatal("executable op not decodable", op, err)
		}
		if _, err := Dispatch(context.Background(), request); err != nil {
			t.Fatal("executable op refused", op, err)
		}
	}
	// The dangerous ops must never appear in the advertised executable set,
	// so a caller cannot discover them as a capability.
	for _, op := range Executable() {
		if op == "SEAL" || op == "PURGE" {
			t.Fatal("dangerous op advertised as executable", op)
		}
	}
}

func TestStatusReportsPurgeReleaseRefused(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"op":"STATUS"}`))
	if err != nil {
		t.Fatal("status not decodable", err)
	}
	value, err := Dispatch(context.Background(), request)
	if err != nil {
		t.Fatal("status refused", err)
	}
	status, ok := value.(Status)
	if !ok {
		t.Fatal("status type mismatch")
	}
	if status.PurgeReleaseAllowed {
		t.Fatal("worker reported purge release as allowed")
	}
}

func TestDispatchRefusesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, _ := DecodeRequest([]byte(`{"op":"PING"}`))
	if _, err := Dispatch(ctx, request); !errors.Is(err, ErrProtocol) {
		t.Fatal("cancelled context accepted", err)
	}
}
