// Command operator-probe is C04-A's client inside the test container. It runs
// as whichever principal the contract names, speaks to the operator socket, and
// prints one JSON result on stdout (or to -out).
//
// It is the contract's instrument, not the thing under test: ADR-0010 § 4
// requires the SERVER to be the production binary, and it is. The probe is
// test code, which is why it lives under test/ and nothing in the product
// imports it.
package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"go.keystone-core.io/keystone-core/test/contract/operator/sockprobe"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: operator-probe <stat|session|multi|listen|bind-stale|kill-on-fifo|read-records|inet-sockets|vm|whoami> [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	var result any
	var out string
	switch cmd {
	case "stat":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		path := fs.String("path", "", "")
		fs.Parse(args)
		result = statPath(*path)
	case "session", "multi":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		path := fs.String("path", "", "socket path")
		frame := fs.String("frame", "", "JSON body, framed and sent one byte per write")
		raw := fs.String("raw-hex", "", "raw bytes, sent one byte per write instead of -frame")
		watch := fs.Int("watch-ms", 3000, "how long to observe each connection")
		waitFor := fs.String("wait-for", "", "block until this file exists before connecting")
		fifo := fs.String("notify-fifo", "", "write one byte to this FIFO when the first frame arrives")
		count := fs.Int("count", 1, "multi: connections")
		stagger := fs.Int("stagger-ms", 0, "multi: delay between connections")
		until := fs.Int("until-frames", 0, "stop observing once this many frames have arrived")
		fs.StringVar(&out, "out", "", "write the result here instead of stdout")
		fs.Parse(args)
		payload := mustPayload(*frame, *raw)
		if *waitFor != "" {
			for {
				if _, err := os.Stat(*waitFor); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		if cmd == "session" {
			result = session(*path, payload, *watch, *until, *fifo)
		} else {
			result = multi(*path, payload, *watch, *until, *count, *stagger)
		}
	case "listen":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		path := fs.String("path", "", "")
		fs.Parse(args)
		fd, err := sockprobe.Listen(*path)
		if err != nil {
			fail("listen: " + err.Error())
		}
		// Accept and hold every connection without reading: a live server that
		// never consumes anything, so a connection to it is observable and a
		// removed socket is not.
		for {
			if _, _, err := syscall.Accept(fd); err != nil {
				fail("accept: " + err.Error())
			}
		}
	case "bind-stale":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		path := fs.String("path", "", "")
		fs.Parse(args)
		fd, err := sockprobe.Listen(*path)
		if err != nil {
			fail("bind: " + err.Error())
		}
		// Closed without unlinking: the file a crashed server leaves behind,
		// which refuses every connection with ECONNREFUSED.
		syscall.Close(fd)
		result = map[string]string{"stale": *path}
	case "kill-on-fifo":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		fifo := fs.String("fifo", "", "")
		pid := fs.Int("pid", 0, "")
		fs.StringVar(&out, "out", "", "")
		fs.Parse(args)
		result = killOnFIFO(*fifo, *pid)
	case "read-records":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		db := fs.String("db", "", "")
		query := fs.String("query", "", "the query the contract freezes")
		fs.Parse(args)
		result = readRecords(*db, *query)
	case "inet-sockets":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		pid := fs.Int("pid", 0, "")
		fs.Parse(args)
		result = inetSockets(*pid)
	case "vm":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		pid := fs.Int("pid", 0, "")
		fs.Parse(args)
		result = vm(*pid)
	case "whoami":
		result = whoami()
	default:
		fail("unknown command " + cmd)
	}
	b, err := json.Marshal(result)
	if err != nil {
		fail(err.Error())
	}
	if out != "" {
		tmp := out + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			fail(err.Error())
		}
		if err := os.Rename(tmp, out); err != nil {
			fail(err.Error())
		}
		return
	}
	fmt.Println(string(b))
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "operator-probe: "+msg)
	os.Exit(2)
}

func mustPayload(frame, raw string) []byte {
	switch {
	case frame != "" && raw != "":
		fail("-frame and -raw-hex are exclusive")
	case frame != "":
		return sockprobe.Frame([]byte(frame))
	case raw != "":
		b, err := hex.DecodeString(raw)
		if err != nil {
			fail("-raw-hex: " + err.Error())
		}
		return b
	}
	return nil
}

type identity struct {
	UID    int   `json:"uid"`
	Groups []int `json:"groups"`
}

func whoami() identity {
	g, _ := syscall.Getgroups()
	return identity{UID: syscall.Getuid(), Groups: g}
}

type statResult struct {
	Errno string `json:"errno"`
	UID   int    `json:"uid"`
	GID   int    `json:"gid"`
	Perm  string `json:"perm"`
	Type  string `json:"type"`
	Inode uint64 `json:"inode"`
}

func statPath(path string) statResult {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return statResult{Errno: sockprobe.ErrnoName(err)}
	}
	t := "other"
	switch st.Mode & syscall.S_IFMT {
	case syscall.S_IFSOCK:
		t = "socket"
	case syscall.S_IFDIR:
		t = "dir"
	case syscall.S_IFREG:
		t = "file"
	}
	return statResult{UID: int(st.Uid), GID: int(st.Gid), Perm: fmt.Sprintf("%04o", st.Mode&0o7777), Type: t, Inode: st.Ino}
}

type sessionResult struct {
	identity
	ConnectErrno string `json:"connect_errno"`
	Sent         int    `json:"sent"`
	sockprobe.Watch
	ProbeError string `json:"probe_error,omitempty"`
}

func session(path string, payload []byte, watchMS, until int, fifo string) sessionResult {
	r := sessionResult{identity: whoami()}
	fd, err := sockprobe.Dial(path)
	if err != nil {
		r.ConnectErrno = sockprobe.ErrnoName(err)
		return r
	}
	defer syscall.Close(fd)
	return observe(fd, r, payload, watchMS, until, fifo)
}

func observe(fd int, r sessionResult, payload []byte, watchMS, until int, fifo string) sessionResult {
	if err := sockprobe.SendSingly(fd, payload); err != nil {
		// A server may close a connection it refused before every byte is
		// written. That is an observation, not a probe failure.
		r.ProbeError = "send: " + sockprobe.ErrnoName(err)
	}
	r.Sent = len(payload)
	var onFrame func()
	if fifo != "" {
		onFrame = func() {
			f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
			if err == nil {
				f.Write([]byte{1})
				f.Close()
			}
		}
	}
	w, err := sockprobe.Observe(fd, time.Duration(watchMS)*time.Millisecond, until, onFrame)
	r.Watch = w
	if err != nil {
		r.ProbeError = err.Error()
	}
	return r
}

// multi opens count connections, stagger apart, and observes them all
// concurrently. Each stays open until its own budget ends or the server closes
// it, so a limit on concurrent connections is exercised by connections that are
// genuinely concurrent.
func multi(path string, payload []byte, watchMS, until, count, stagger int) []sessionResult {
	out := make([]sessionResult, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		if i > 0 && stagger > 0 {
			time.Sleep(time.Duration(stagger) * time.Millisecond)
		}
		r := sessionResult{identity: whoami()}
		fd, err := sockprobe.Dial(path)
		if err != nil {
			r.ConnectErrno = sockprobe.ErrnoName(err)
			out[i] = r
			continue
		}
		wg.Add(1)
		go func(i, fd int, r sessionResult) {
			defer wg.Done()
			defer syscall.Close(fd)
			out[i] = observe(fd, r, payload, watchMS, until, "")
		}(i, fd, r)
	}
	wg.Wait()
	return out
}

// killOnFIFO blocks until a writer opens the FIFO, then SIGKILLs pid. The
// session that writes it does so the moment a response frame has arrived, so
// the server is killed microseconds after answering -- inside the window a
// server that answered before its record was durable would lose the record.
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
	return map[string]string{"killed": strconv.Itoa(pid)}
}

type auditResult struct {
	Error   string           `json:"error,omitempty"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// readRecords runs the contract's query, generically. The probe knows no table
// or column the contract asserts on; the contract does, from its frozen surface.
func readRecords(db, query string) auditResult {
	var r auditResult
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

type socketsResult struct {
	Error string   `json:"error,omitempty"`
	Inet  []string `json:"inet"`
	Unix  int      `json:"unix"`
}

// inetSockets lists every IP socket pid holds, by matching the socket inodes
// behind its descriptors against its network namespace's tables.
func inetSockets(pid int) socketsResult {
	var r socketsResult
	fds, err := filepath.Glob(fmt.Sprintf("/proc/%d/fd/*", pid))
	if err != nil || len(fds) == 0 {
		r.Error = fmt.Sprintf("no descriptors readable for pid %d: %v", pid, err)
		return r
	}
	held := map[string]bool{}
	for _, fd := range fds {
		l, err := os.Readlink(fd)
		if err == nil && strings.HasPrefix(l, "socket:[") {
			held[strings.TrimSuffix(strings.TrimPrefix(l, "socket:["), "]")] = true
		}
	}
	for _, table := range []string{"tcp", "tcp6", "udp", "udp6", "raw", "raw6"} {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/%s", pid, table))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				r.Error = err.Error()
				return r
			}
			continue
		}
		for _, line := range strings.Split(string(b), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) > 9 && held[f[9]] {
				r.Inet = append(r.Inet, table+":"+f[9])
			}
		}
	}
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/unix", pid)); err == nil {
		for _, line := range strings.Split(string(b), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) > 6 && held[f[6]] {
				r.Unix++
			}
		}
	}
	return r
}

type vmResult struct {
	Error    string `json:"error,omitempty"`
	VmPeakKB int    `json:"vm_peak_kb"`
}

// vm reports the peak virtual size of pid. An allocation the size of a hostile
// length prefix shows here even when Go never touches its pages, which is why
// the contract reads this rather than a memory limit: a lazily backed 4 GiB
// slice survives a cgroup cap that it would never actually reach.
func vm(pid int) vmResult {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return vmResult{Error: err.Error()}
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "VmPeak:" {
			n, err := strconv.Atoi(f[1])
			if err != nil {
				return vmResult{Error: err.Error()}
			}
			return vmResult{VmPeakKB: n}
		}
	}
	return vmResult{Error: "no VmPeak line"}
}
