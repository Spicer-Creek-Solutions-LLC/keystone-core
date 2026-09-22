// Package sockprobe is the instrument C04-A's contract measures the operator
// socket with. It is test code: nothing in the product imports it.
//
// It uses syscall rather than net. Nothing here needs net, and
// internal/cli/boundary_test.go reads every file in the module, test/ included;
// a probe that imported net would have to loosen a boundary the product is
// still held to.
//
// # The ordering instrument
//
// ADR-0009 § 3 requires that no request byte is read before the server has
// decided whether the peer is authorized. A refusal proves the outcome and not
// the order, so the contract measures the order from outside the server:
// SIOCOUTQ on a unix stream socket reports the bytes the client sent that the
// PEER HAS NOT CONSUMED YET. An AF_UNIX skb stays charged to the sender until
// the receiver has read all of it.
//
// Two properties of that count decide how it is used, and sockprobe_test.go
// asserts both on the running kernel rather than trusting this comment:
//
//   - The count moves only when an skb is FULLY consumed. A server that reads
//     one byte of a hundred-byte write leaves it unchanged. SendSingly
//     therefore writes one byte per syscall, so that a read of any single byte
//     consumes a whole skb and moves the count.
//   - MSG_PEEK consumes nothing, and the count cannot see it. That is a limit
//     of this instrument, stated in C04-A's evidence rather than hidden.
//
// The count is in units of skb truesize, not payload bytes. Every assertion
// is therefore about whether the count CHANGED, never about its value.
package sockprobe

import (
	"encoding/binary"
	"errors"
	"syscall"
	"time"
	"unsafe"
)

// Dial connects a unix stream socket to path. The descriptor is blocking.
func Dial(path string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// Listen binds and listens on path. It is the probe's own server, used to
// prove the instrument and to plant a live or stale socket.
func Listen(path string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
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

// OutQ reports the send-queue count the peer has not consumed.
func OutQ(fd int) (int, error) {
	var n int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCOUTQ), uintptr(unsafe.Pointer(&n))); e != 0 {
		return 0, e
	}
	return int(n), nil
}

// SendSingly writes b one byte per syscall. See the package comment for why.
func SendSingly(fd int, b []byte) error {
	for i := range b {
		if _, err := syscall.Write(fd, b[i:i+1]); err != nil {
			return err
		}
	}
	return nil
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

// Sample is one observation of the send queue, in milliseconds since the
// watch began.
type Sample struct {
	MS   int `json:"ms"`
	OutQ int `json:"outq"`
}

// Watch is what one connection observed after its payload was sent.
type Watch struct {
	// InitialOutQ is the count once every payload byte was written and before
	// the first sample. Zero means the payload was empty or already consumed.
	InitialOutQ int `json:"initial_outq"`
	// Samples records each CHANGE of the count, not every poll.
	Samples []Sample `json:"samples"`
	// FirstDropMS is when the count first fell below InitialOutQ, or -1.
	FirstDropMS int `json:"first_drop_ms"`
	// Frames are the complete response bodies, in arrival order.
	Frames   []string `json:"frames"`
	FrameMS  []int    `json:"frame_ms"`
	Trailing int      `json:"trailing_bytes"`
	// EOFMS is when the server closed the connection, or -1.
	EOFMS int `json:"eof_ms"`
}

// ErrWatch wraps a failure of the instrument itself, so a caller can tell a
// broken probe from an observed server behaviour.
var ErrWatch = errors.New("sockprobe: watch failed")

// Observe samples fd's send queue and reads responses until the server closes
// the connection, budget elapses, or -- when untilFrames is positive -- that
// many complete frames have arrived. onFrame, when set, runs once, after the
// first complete frame arrives.
func Observe(fd int, budget time.Duration, untilFrames int, onFrame func()) (Watch, error) {
	w := Watch{FirstDropMS: -1, EOFMS: -1}
	q, err := OutQ(fd)
	if err != nil {
		return w, errors.Join(ErrWatch, err)
	}
	w.InitialOutQ = q
	last := q
	if err := syscall.SetNonblock(fd, true); err != nil {
		return w, errors.Join(ErrWatch, err)
	}
	start := time.Now()
	var buf []byte
	chunk := make([]byte, 4096)
	notified := false
	for {
		ms := int(time.Since(start) / time.Millisecond)
		if q, err := OutQ(fd); err == nil {
			if q != last {
				w.Samples = append(w.Samples, Sample{MS: ms, OutQ: q})
				last = q
			}
			if q < w.InitialOutQ && w.FirstDropMS < 0 {
				w.FirstDropMS = ms
			}
		}
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
			w.EOFMS = ms
			w.Trailing = len(buf)
			return w, nil
		case err == syscall.EAGAIN:
		case err == syscall.ECONNRESET:
			w.EOFMS = ms
			w.Trailing = len(buf)
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
	case syscall.EPERM:
		return "EPERM"
	case syscall.ENOTDIR:
		return "ENOTDIR"
	}
	return e.Error()
}
