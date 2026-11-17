package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"syscall"
)

// ErrServiceDown means nothing listens on the socket: no file, or a stale one.
var ErrServiceDown = errors.New("service not running")

// Call sends one request and reads the one response.
func Call(ctx context.Context, path string, req Request) (Response, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return Response{}, fmt.Errorf("%s: %w", path, ErrServiceDown)
		}
		return Response{}, fmt.Errorf("cannot connect to %s: %w", path, err)
	}
	defer conn.Close()                                      //nolint:errcheck // the response is already read or the call already failed
	stop := context.AfterFunc(ctx, func() { conn.Close() }) //nolint:errcheck // unblocks the read on cancel
	defer stop()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, fmt.Errorf("cannot send %s: %w", req.Verb, err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("cannot read the answer to %s: %w", req.Verb, err)
	}
	return resp, nil
}
