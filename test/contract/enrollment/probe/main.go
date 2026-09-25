// Command enrollment-probe is C05's instrument inside the test container. It
// runs as whichever principal the contract names and prints one JSON result.
//
// It is test code, not the thing under test: the server and the CLI are the
// production binaries. It uses syscall rather than net, for the reason
// test/contract/operator/sockprobe gives.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	_ "modernc.org/sqlite"

	"go.keystone-core.io/keystone-core/test/contract/operator/sockprobe"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: enrollment-probe <read-records|pipeline|request-and-notify|kill-on-fifo|watch-opens|connect> [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var result any
	var out string
	switch cmd {
	case "read-records":
		db := fs.String("db", "", "")
		query := fs.String("query", "", "")
		fs.Parse(args)
		result = readRecords(*db, *query)
	case "pipeline":
		path := fs.String("path", "", "")
		first := fs.String("first", "", "JSON body of the first request")
		second := fs.String("second", "", "JSON body of the second request")
		watch := fs.Int("watch-ms", 20000, "")
		fs.Parse(args)
		result = pipeline(*path, []byte(*first), []byte(*second), time.Duration(*watch)*time.Millisecond)
	case "request-and-notify":
		path := fs.String("path", "", "")
		frame := fs.String("frame", "", "")
		fifo := fs.String("fifo", "", "written the moment the response is complete")
		fs.Parse(args)
		result = requestAndNotify(*path, []byte(*frame), *fifo)
	case "kill-on-fifo":
		fifo := fs.String("fifo", "", "")
		pid := fs.Int("pid", 0, "")
		fs.StringVar(&out, "out", "", "")
		fs.Parse(args)
		result = killOnFIFO(*fifo, *pid)
	case "watch-opens":
		dir := fs.String("dir", "", "")
		ready := fs.String("ready", "", "created once the watch is in place")
		until := fs.String("until", "", "stop once this file exists")
		fs.StringVar(&out, "out", "", "")
		fs.Parse(args)
		result = watchOpens(*dir, *ready, *until)
	case "connect":
		path := fs.String("path", "", "")
		frame := fs.String("frame", "", "")
		fs.Parse(args)
		result = connect(*path, []byte(*frame))
	default:
		fail("unknown command " + cmd)
	}
	b, err := json.Marshal(result)
	if err != nil {
		fail(err.Error())
	}
	if out != "" {
		if err := os.WriteFile(out+".tmp", b, 0o644); err != nil {
			fail(err.Error())
		}
		if err := os.Rename(out+".tmp", out); err != nil {
			fail(err.Error())
		}
		return
	}
	fmt.Println(string(b))
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "enrollment-probe: "+msg)
	os.Exit(2)
}

type records struct {
	Error   string           `json:"error,omitempty"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// readRecords runs a query read-only, as a separate connection to the server's
// store, and returns every column generically.
func readRecords(db, query string) records {
	var r records
	conn, err := sql.Open("sqlite", "file:"+db+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer conn.Close()
	rows, err := conn.Query(query)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer rows.Close()
	r.Columns, _ = rows.Columns()
	for rows.Next() {
		vals := make([]any, len(r.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			r.Error = err.Error()
			return r
		}
		row := map[string]any{}
		for i, c := range r.Columns {
			if b, ok := vals[i].([]byte); ok {
				row[c] = string(b)
			} else {
				row[c] = vals[i]
			}
		}
		r.Rows = append(r.Rows, row)
	}
	if err := rows.Err(); err != nil {
		r.Error = err.Error()
	}
	return r
}

// outq is SIOCOUTQ on a unix stream socket: the memory still held by what this
// end sent and the peer has not consumed. Linux frees a sent buffer only when
// the peer has read all of it, so the value falls exactly when the peer reads.
func outq(fd int) (int, error) {
	const siocoutq = 0x5411 // TIOCOUTQ
	var n int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), siocoutq, uintptr(unsafe.Pointer(&n))); e != 0 {
		return 0, e
	}
	return int(n), nil
}

type pipelineResult struct {
	Error string `json:"error,omitempty"`
	// FirstConsumed: the server read the whole first request.
	FirstConsumed bool `json:"first_consumed"`
	// Pending is SIOCOUTQ once the second request was sent, one byte per
	// write, so that reading any byte of it frees a buffer and lowers it.
	Pending int `json:"pending"`
	// Samples is how many times SIOCOUTQ was read before the first reply was
	// complete.
	Samples int `json:"samples"`
	// ReadEarly: some sample, taken before the first reply had arrived, showed
	// the second request partly or wholly consumed.
	ReadEarly bool     `json:"read_early"`
	Frames    []string `json:"frames"`
	FrameMS   []int    `json:"frame_ms"`
	// SecondConsumed: after the first reply, the server went on to read the
	// second request -- the control that the instrument can see a read at all.
	SecondConsumed bool `json:"second_consumed"`
}

// pipeline sends a first request, waits until the server has consumed it, then
// sends a second without waiting for the first reply, and watches whether the
// server reads the second before answering the first.
//
// Each iteration reads SIOCOUTQ BEFORE reading the socket. A server that writes
// its reply and then reads the next request has put the reply in this end's
// queue before the read that lowers SIOCOUTQ, so the read after a low sample
// always finds the whole reply. A low sample with no complete reply behind it
// is therefore a read of the second request before the first was answered.
func pipeline(path string, first, second []byte, budget time.Duration) pipelineResult {
	var r pipelineResult
	fd, err := sockprobe.Dial(path)
	if err != nil {
		r.Error = "dial: " + sockprobe.ErrnoName(err)
		return r
	}
	defer syscall.Close(fd)
	if err := writeAll(fd, sockprobe.Frame(first)); err != nil {
		r.Error = "send first: " + err.Error()
		return r
	}
	start := time.Now()
	for time.Since(start) < budget {
		q, err := outq(fd)
		if err != nil {
			r.Error = "SIOCOUTQ: " + err.Error()
			return r
		}
		if q == 0 {
			r.FirstConsumed = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !r.FirstConsumed {
		r.Error = "the server never consumed the first request"
		return r
	}
	for _, b := range sockprobe.Frame(second) {
		if err := writeAll(fd, []byte{b}); err != nil {
			r.Error = "send second: " + err.Error()
			return r
		}
	}
	if r.Pending, err = outq(fd); err != nil || r.Pending == 0 {
		r.Error = fmt.Sprintf("the second request is not observable as pending: %d %v", r.Pending, err)
		return r
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		r.Error = err.Error()
		return r
	}
	var buf []byte
	chunk := make([]byte, 64<<10)
	for time.Since(start) < budget && len(r.Frames) < 2 {
		q, err := outq(fd)
		if err != nil {
			r.Error = "SIOCOUTQ: " + err.Error()
			return r
		}
		for {
			n, err := syscall.Read(fd, chunk)
			if n > 0 {
				buf = append(buf, chunk[:n]...)
				continue
			}
			if err == syscall.EAGAIN || n == 0 {
				break
			}
			r.Error = "read: " + err.Error()
			return r
		}
		var bodies [][]byte
		bodies, buf = sockprobe.Frames(buf)
		for _, b := range bodies {
			r.Frames = append(r.Frames, string(b))
			r.FrameMS = append(r.FrameMS, int(time.Since(start)/time.Millisecond))
		}
		if len(r.Frames) == 0 {
			r.Samples++
			if q < r.Pending {
				r.ReadEarly = true
			}
		}
		if len(r.Frames) >= 1 && q == 0 {
			r.SecondConsumed = true
		}
		time.Sleep(time.Millisecond)
	}
	return r
}

func writeAll(fd int, b []byte) error {
	for len(b) > 0 {
		n, err := syscall.Write(fd, b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

type notifyResult struct {
	Error string `json:"error,omitempty"`
	Frame string `json:"frame"`
}

// requestAndNotify sends one request and writes the FIFO the instant the
// response is complete. kill-on-fifo, as root, is blocked on the other end and
// SIGKILLs the server: a server that answered before its record was durable
// would lose the record in that window.
func requestAndNotify(path string, frame []byte, fifo string) notifyResult {
	var r notifyResult
	fd, err := sockprobe.Dial(path)
	if err != nil {
		r.Error = "dial: " + sockprobe.ErrnoName(err)
		return r
	}
	defer syscall.Close(fd)
	if err := writeAll(fd, sockprobe.Frame(frame)); err != nil {
		r.Error = err.Error()
		return r
	}
	var buf []byte
	chunk := make([]byte, 64<<10)
	for {
		n, err := syscall.Read(fd, chunk)
		if n <= 0 {
			r.Error = fmt.Sprintf("closed before a response: %v", err)
			return r
		}
		buf = append(buf, chunk[:n]...)
		if bodies, _ := sockprobe.Frames(buf); len(bodies) > 0 {
			if f, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
				f.Write([]byte{1})
				f.Close()
			}
			r.Frame = string(bodies[0])
			return r
		}
	}
}

func killOnFIFO(fifo string, pid int) map[string]string {
	f, err := os.Open(fifo)
	if err != nil {
		fail("open fifo: " + err.Error())
	}
	f.Read(make([]byte, 1))
	f.Close()
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return map[string]string{"kill_error": err.Error()}
	}
	return map[string]string{"killed": fmt.Sprint(pid)}
}

type opensResult struct {
	Error  string   `json:"error,omitempty"`
	Opened []string `json:"opened"`
}

// watchOpens records every file opened in dir, by inotify, until the until
// file exists.
func watchOpens(dir, ready, until string) opensResult {
	var r opensResult
	fd, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer syscall.Close(fd)
	if _, err := syscall.InotifyAddWatch(fd, dir, syscall.IN_OPEN); err != nil {
		r.Error = err.Error()
		return r
	}
	if err := os.WriteFile(ready, nil, 0o644); err != nil {
		r.Error = err.Error()
		return r
	}
	seen := map[string]bool{}
	buf := make([]byte, 64<<10)
	for {
		stop := false
		if _, err := os.Stat(until); err == nil {
			stop = true
		}
		n, err := syscall.Read(fd, buf)
		for off := 0; n > 0 && off+syscall.SizeofInotifyEvent <= n; {
			ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := strings.TrimRight(string(buf[off+syscall.SizeofInotifyEvent:off+syscall.SizeofInotifyEvent+int(ev.Len)]), "\x00")
			if name != "" && !seen[name] {
				seen[name] = true
				r.Opened = append(r.Opened, name)
			}
			off += syscall.SizeofInotifyEvent + int(ev.Len)
		}
		if err != nil && err != syscall.EAGAIN {
			r.Error = err.Error()
			return r
		}
		if stop {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type connectResult struct {
	ConnectErrno string   `json:"connect_errno"`
	Frames       []string `json:"frames"`
}

// connect reports how the socket treats this principal: refused by the
// kernel, or connected and answered.
func connect(path string, frame []byte) connectResult {
	var r connectResult
	fd, err := sockprobe.Dial(path)
	if err != nil {
		r.ConnectErrno = sockprobe.ErrnoName(err)
		return r
	}
	defer syscall.Close(fd)
	writeAll(fd, sockprobe.Frame(frame))
	w, _ := sockprobe.Observe(fd, 5*time.Second, 1, nil)
	r.Frames = w.Frames
	return r
}
