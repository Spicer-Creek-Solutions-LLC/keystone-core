//go:build contract

package operatorcontract

import (
	"strings"
	"testing"
)

// TestFixtureInstrumentsTheHost is not an acceptance case and is not pending.
// It runs on every `make contract` and proves the parts of the fixture that do
// not need C04-I's server: the image, the principals, the instrument inside the
// container, and the premise the stale-member cases rest on.
//
// C03-A's evidence had to say its fixture was "reviewed but not run" until
// C03-I, and C03-I then corrected it. Everything here was runnable at C04-A, so
// it runs at C04-A.
func TestFixtureInstrumentsTheHost(t *testing.T) {
	b := newBox(t)

	// A listener standing in for the server: the ADR-0009 § 1 directory and
	// socket modes, and a peer that accepts and never reads.
	b.plantDir("0750")
	b.detach("", probeBin, "listen", "-path", socketPath)
	for i := 0; i < 50 && b.stat("", socketPath).Type != "socket"; i++ {
		b.exec("", "sleep", "0.1")
	}
	b.sh("chgrp " + adminGroup + " " + socketPath + " && chmod 0660 " + socketPath)

	t.Run("a member reaches the socket and it stays open", func(t *testing.T) {
		s := b.session(alice, "-frame", request(controlOp, nil), "-watch-ms", "300")
		if s.ConnectErrno != "" || s.Ended != endedOpen {
			t.Fatalf("against a peer that holds the connection: %+v", s)
		}
	})

	// ORD-1's instrument, inside the container: a peer that closes without
	// reading must be seen to have left the byte unconsumed.
	t.Run("a peer that closes without reading is seen not to have read", func(t *testing.T) {
		path := socketDir + "/closer.sock"
		b.detach("", probeBin, "listen", "-path", path, "-close-after-ms", "200")
		for i := 0; i < 50 && b.stat("", path).Type != "socket"; i++ {
			b.exec("", "sleep", "0.1")
		}
		b.sh("chgrp " + adminGroup + " " + path + " && chmod 0660 " + path)
		var s session
		b.probe(alice, &s, "session", "-path", path, "-raw-hex", oneByte, "-watch-ms", "3000")
		if !s.ConsumptionObservable || s.ConnectErrno != "" || s.Ended != endedReset {
			t.Fatalf("a close on an unread byte was not observed as unread: %+v", s)
		}
	})

	t.Run("the kernel refuses an outsider", func(t *testing.T) {
		if st := b.stat(outsider, socketPath); st.Errno != "EACCES" {
			t.Fatalf("stat: %+v", st)
		}
		if s := b.session(outsider, "-watch-ms", "100"); s.ConnectErrno != "EACCES" {
			t.Fatalf("connect: %+v", s)
		}
	})

	// The premise of every stale-member case: a session started while stale was
	// a member keeps the group after the membership is revoked, so the kernel
	// admits it and only an in-band check can refuse it.
	t.Run("a revoked member's running session still passes the kernel", func(t *testing.T) {
		release := b.armStale("selftest", "-frame", request(controlOp, nil), "-watch-ms", "300")
		b.revoke(stale)
		s := release()
		if s.ConnectErrno != "" {
			t.Fatalf("the kernel refused the stale session: %+v", s)
		}
		var fresh struct {
			Groups []int `json:"groups"`
		}
		b.probe(stale, &fresh, "whoami")
		if hasGroup(fresh.Groups, adminGID) {
			t.Fatalf("a session started after revocation still has group %d: %v", adminGID, fresh.Groups)
		}
		b.grant(stale)
	})

	t.Run("the listener holds no IP socket", func(t *testing.T) {
		pid := strings.Fields(b.sh("pgrep -f '^" + probeBin + " listen -path " + socketPath + "$'"))
		if len(pid) != 1 {
			t.Fatalf("listener pids: %v", pid)
		}
		var r struct {
			Error string   `json:"error"`
			Inet  []string `json:"inet"`
			Unix  int      `json:"unix"`
		}
		b.probe("", &r, "inet-sockets", "-pid", pid[0])
		if r.Error != "" || len(r.Inet) != 0 || r.Unix == 0 {
			t.Fatalf("%+v", r)
		}
	})

	t.Run("a planted stale socket refuses with ECONNREFUSED", func(t *testing.T) {
		path := socketDir + "/stale.sock"
		var r map[string]string
		b.probe("", &r, "bind-stale", "-path", path)
		var s session
		b.probe("", &s, "session", "-path", path, "-watch-ms", "100")
		if s.ConnectErrno != "ECONNREFUSED" {
			t.Fatalf("%+v", s)
		}
	})

	t.Run("kill-on-fifo kills its target when a session notifies it", func(t *testing.T) {
		b.detach("", "sleep", "300")
		pid := strings.Fields(b.sh("pgrep -x sleep"))
		if len(pid) != 1 {
			t.Fatalf("sleep pids: %v", pid)
		}
		b.sh("mkfifo -m 0666 /tmp/selftest.fifo")
		b.detach("", probeBin, "kill-on-fifo", "-fifo", "/tmp/selftest.fifo", "-pid", pid[0], "-out", "/tmp/selftest.kill")
		// A member's session against the listener gets no frame, so the FIFO is
		// written directly here; ORD-2 has the session write it.
		b.sh("echo x > /tmp/selftest.fifo")
		for i := 0; i < 50; i++ {
			if out, err := b.exec("", "cat", "/tmp/selftest.kill"); err == nil && strings.Contains(out, "killed") {
				if _, err := b.exec("", "kill", "-0", pid[0]); err == nil {
					t.Fatalf("pid %s survived", pid[0])
				}
				return
			}
			b.exec("", "sleep", "0.1")
		}
		t.Fatal("kill-on-fifo reported nothing")
	})

	t.Run("the peak virtual size of a process is readable", func(t *testing.T) {
		pid := strings.Fields(b.sh("pgrep -f '^" + probeBin + " listen -path " + socketPath + "$'"))
		var r struct {
			Error    string `json:"error"`
			VmPeakKB int    `json:"vm_peak_kb"`
		}
		b.probe("", &r, "vm", "-pid", pid[0])
		if r.Error != "" || r.VmPeakKB <= 0 {
			t.Fatalf("%+v", r)
		}
	})
}
