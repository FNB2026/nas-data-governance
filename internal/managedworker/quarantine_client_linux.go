//go:build linux

package managedworker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"time"
)

// QuarantineClient connects to a configured worker socket. The Web process
// holds no project store and checks the server's kernel peer identity.
type QuarantineClient struct {
	RunDir string
	Worker Subject
}

func (c QuarantineClient) Call(ctx context.Context, op, item, plan, digest, confirmation string) (QuarantinePurgeResult, string, error) {
	var empty QuarantinePurgeResult
	message := map[string]string{"op": op}
	switch op {
	case "QUARANTINE_PURGE_PLAN":
		message["item_id"] = item
	case "QUARANTINE_PURGE_EXECUTE":
		message["confirmation"] = confirmation
		fallthrough
	case "QUARANTINE_PURGE_APPROVE", "QUARANTINE_PURGE_DRY_RUN", "QUARANTINE_PURGE_RECOVER", "QUARANTINE_PURGE_RECEIPT":
		message["plan_id"], message["digest"] = plan, digest
	default:
		return empty, "", ErrProtocol
	}
	if c.Worker.UID == 0 || c.Worker.GID == 0 || !filepath.IsAbs(c.RunDir) || filepath.Clean(c.RunDir) != c.RunDir {
		return empty, "", ErrIsolation
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return empty, "", ErrProtocol
	}
	request, err := frame(raw)
	if err != nil {
		return empty, "", err
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", filepath.Join(c.RunDir, SocketName))
	if err != nil {
		return empty, "", ErrIsolation
	}
	defer conn.Close()
	peer, err := peerSubject(conn.(*net.UnixConn))
	if err != nil || peer != c.Worker {
		return empty, "", ErrUnauthorized
	}
	deadline := time.Now().Add(5 * time.Minute)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if conn.SetDeadline(deadline) != nil {
		return empty, "", ErrProtocol
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if n, err := conn.Write(request); err != nil || n != len(request) {
		return empty, "", ErrProtocol
	}
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return empty, "", ErrProtocol
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxRequestBytes {
		return empty, "", ErrProtocol
	}
	raw = make([]byte, size)
	if _, err := io.ReadFull(conn, raw); err != nil {
		return empty, "", ErrProtocol
	}
	var response quarantineResponse
	if json.Unmarshal(raw, &response) != nil {
		return empty, "", ErrProtocol
	}
	return response.Plan, response.Reason, nil
}
