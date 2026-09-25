package operator

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"
)

// maxResponse bounds a response frame the client will accept. The largest
// response is a token bundle, a few kilobytes.
const maxResponse = 1 << 20

var (
	// ErrUnauthorized is the one outcome of both authorization layers: the
	// kernel refusing connect(), and the in-band check denying a connected
	// peer. ADR-0009 § 5 makes them indistinguishable to the caller, and the
	// client keeps them so -- it does not record which one refused.
	ErrUnauthorized = errors.New("authorization denied")
	// ErrNoServer means nothing is listening at the socket path.
	ErrNoServer = errors.New("the keystone server is not running")
	// ErrNoResponse means the server closed the connection without answering.
	// Whether it acted on the request is not known.
	ErrNoResponse = errors.New("the keystone server closed the connection without a response")
)

// ServerError is an error frame other than an authorization denial.
type ServerError struct{ Code string }

func (e *ServerError) Error() string { return "the keystone server refused the request: " + e.Code }

// Call sends one request over the operator socket at path and decodes the one
// response into out.
func Call(path string, request, out any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		switch {
		case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
			return ErrUnauthorized
		case errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.ENOTDIR):
			return ErrNoServer
		}
		return fmt.Errorf("connect %s: %w", path, err)
	}
	defer conn.Close()
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	// A write error is not the outcome. A peer the in-band check refused is
	// answered and closed without its request being read, so the write can
	// fail with EPIPE while the denial is already waiting to be read.
	_, _ = conn.Write(append(prefix[:], body...))

	if _, err := io.ReadFull(conn, prefix[:]); err != nil {
		return ErrNoResponse
	}
	n := binary.BigEndian.Uint32(prefix[:])
	if n > maxResponse {
		return fmt.Errorf("response of %d bytes exceeds %d", n, maxResponse)
	}
	resp := make([]byte, n)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return ErrNoResponse
	}
	var e map[string]json.RawMessage
	if json.Unmarshal(resp, &e) == nil && len(e) == 1 {
		if raw, ok := e["error"]; ok {
			var code string
			if err := json.Unmarshal(raw, &code); err != nil {
				return fmt.Errorf("malformed error response: %w", err)
			}
			if code == ErrAuthorizationDenied {
				return ErrUnauthorized
			}
			return &ServerError{Code: code}
		}
	}
	return json.Unmarshal(resp, out)
}
