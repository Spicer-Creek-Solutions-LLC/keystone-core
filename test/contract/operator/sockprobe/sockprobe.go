// Package sockprobe is the instrument C04-A's contract speaks to the operator
// socket with. It is test code: nothing in the product imports it.
//
// It uses syscall rather than net. Nothing here needs net, and
// internal/cli/boundary_test.go reads every file in the module, test/ included;
// a probe that imported net would have to loosen a boundary the product is
// still held to.
//
// # How a connection ended, and what that shows
//
// C04-A freezes a platform-neutral requirement: a connection the in-band check
// refuses has none of its request bytes consumed. Observing that from outside
// the server is platform-specific, and is isolated in ConsumptionObservable,
// which each platform's file defines. See unread_linux.go for the one platform
// with an instrument today.
package sockprobe

import (
	"encoding/binary"
	"errors"
	"syscall"
	"time"
)

// Dial connects a unix stream socket to path. The descriptor is blocking.
func Dial(path string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	syscall.CloseOnExec(fd)
	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// Listen binds and listens on path. It is the probe's own server, used to
// prove the instrument and to plant a live or stale socket.
func Listen(path string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	syscall.CloseOnExec(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if err := syscall.Listen(fd, 16); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// Frame is the operator framing C04-A freezes: a uint32 big-endian length,
// then that many bytes of body.
func Frame(body []byte) []byte {
	out := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(out, uint32(len(body)))
	copy(out[4:], body)
	return out
}

// Frames splits buf into complete frame bodies and returns the unconsumed
// remainder.
func Frames(buf []byte) (bodies [][]byte, rest []byte) {
	for len(buf) >= 4 {
		n := binary.BigEndian.Uint32(buf)
		if uint64(len(buf)-4) < uint64(n) {
			break
		}
		bodies = append(bodies, append([]byte(nil), buf[4:4+n]...))
		buf = buf[4+n:]
	}
	return bodies, buf
}

// How a connection ended, as the client saw it.
const (
	// EndedOpen: the budget ran out, or enough frames arrived, before the
	// server closed the connection.
	EndedOpen = "open"
	// EndedEOF: the server closed the connection and the client read a clean
	// end of stream.
	EndedEOF = "eof"
	// EndedReset: the server closed the connection and the client's read
	// failed with ECONNRESET. What that shows depends on the platform; see
	// ConsumptionObservable.
	EndedReset = "reset"
)

// Watch is what one connection observed after its payload was sent.
type Watch struct {
	// Frames are the complete response bodies, in arrival order.
	Frames   []string `json:"frames"`
	FrameMS  []int    `json:"frame_ms"`
	Trailing int      `json:"trailing_bytes"`
	// Ended is EndedOpen, EndedEOF or EndedReset.
	Ended string `json:"ended"`
	// EOFMS is when the server closed the connection, or -1.
	EOFMS int `json:"eof_ms"`
}

// ErrWatch wraps a failure of the instrument itself, so a caller can tell a
// broken probe from an observed server behaviour.
var ErrWatch = errors.New("sockprobe: watch failed")

// Observe reads responses until the server closes the connection, budget
// elapses, or -- when untilFrames is positive -- that many complete frames
// have arrived. onFrame, when set, runs once, after the first complete frame
// arrives.
//
// Every byte the server wrote is read before the end is classified: data
// queued ahead of a reset is still delivered, so a denial followed by a reset
// is observed as both.
func Observe(fd int, budget time.Duration, untilFrames int, onFrame func()) (Watch, error) {
	w := Watch{Ended: EndedOpen, EOFMS: -1}
	if err := syscall.SetNonblock(fd, true); err != nil {
		return w, errors.Join(ErrWatch, err)
	}
	start := time.Now()
	var buf []byte
	chunk := make([]byte, 4096)
	notified := false
	for {
		ms := int(time.Since(start) / time.Millisecond)
		n, err := syscall.Read(fd, chunk)
		switch {
		case n > 0:
			buf = append(buf, chunk[:n]...)
			var bodies [][]byte
			bodies, buf = Frames(buf)
			for _, b := range bodies {
				w.Frames = append(w.Frames, string(b))
				w.FrameMS = append(w.FrameMS, ms)
			}
			if len(w.Frames) > 0 && !notified && onFrame != nil {
				notified = true
				onFrame()
			}
			if untilFrames > 0 && len(w.Frames) >= untilFrames {
				w.Trailing = len(buf)
				return w, nil
			}
			continue
		case n == 0 && err == nil:
			w.Ended, w.EOFMS, w.Trailing = EndedEOF, ms, len(buf)
			return w, nil
		case err == syscall.EAGAIN:
		case err == syscall.ECONNRESET:
			w.Ended, w.EOFMS, w.Trailing = EndedReset, ms, len(buf)
			return w, nil
		default:
			return w, errors.Join(ErrWatch, err)
		}
		if time.Since(start) >= budget {
			w.Trailing = len(buf)
			return w, nil
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// ErrnoName names the errno values the contract asserts on, so a result
// crosses a process boundary as a stable string rather than a localised
// message.
func ErrnoName(err error) string {
	if err == nil {
		return ""
	}
	var e syscall.Errno
	if !errors.As(err, &e) {
		return err.Error()
	}
	switch e {
	case syscall.EACCES:
		return "EACCES"
	case syscall.ENOENT:
		return "ENOENT"
	case syscall.ECONNREFUSED:
		return "ECONNREFUSED"
	case syscall.ECONNRESET:
		return "ECONNRESET"
	case syscall.EPIPE:
		return "EPIPE"
	case syscall.EPERM:
		return "EPERM"
	case syscall.ENOTDIR:
		return "ENOTDIR"
	}
	return e.Error()
}
