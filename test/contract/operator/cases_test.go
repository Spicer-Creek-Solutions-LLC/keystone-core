//go:build contract

package operatorcontract

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Every refusal case below runs beside authorizedControl on the same server.
// C03-I3 found the shape at the broker and C04.md § 5.3 names it here: a server
// that refuses everyone satisfies every denial.

// ---------------------------------------------------------------------------
// SOCK -- ADR-0009 § 1
// ---------------------------------------------------------------------------

func TestSOCK1SocketOwnershipAndModes(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	for _, want := range []struct {
		path, typ, perm string
	}{{socketDir, "dir", "0750"}, {socketPath, "socket", "0660"}} {
		st := b.stat("", want.path)
		if st.Type != want.typ || st.UID != 0 || st.GID != adminGID || st.Perm != want.perm {
			t.Errorf("%s is %s %d:%d %s; want %s 0:%d %s", want.path, st.Type, st.UID, st.GID, st.Perm, want.typ, adminGID, want.perm)
		}
	}
	b.authorizedControl()
}

func TestSOCK2RefusesToStartWithoutAdminGroup(t *testing.T) {
	b := newBox(t)
	b.refuse(serverConfig{})
	if st := b.stat("", socketPath); st.Errno != "ENOENT" {
		t.Fatalf("a server that refused to start left %s behind: %+v", socketPath, st)
	}
	// The refusal must be the missing group and nothing else.
	b.start(defaultConfig())
	b.authorizedControl()
}

func TestSOCK3OutsiderCannotStatTheSocket(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	if st := b.stat(outsider, socketPath); st.Errno != "EACCES" {
		t.Fatalf("an outsider's stat of the socket returned %+v; the directory must not be traversable", st)
	}
	if st := b.stat(alice, socketPath); st.Type != "socket" {
		t.Fatalf("a member could not stat the socket: %+v", st)
	}
}

func TestSOCK4KernelRefusesOutsiderConnect(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	s := b.session(outsider, "-frame", request(controlOp, nil), "-watch-ms", "2000", "-until-frames", "1")
	if s.ConnectErrno != "EACCES" {
		t.Fatalf("an outsider's connect() returned %q; the kernel must refuse it with EACCES: %+v", s.ConnectErrno, s)
	}
	b.authorizedControl()
}

// ---------------------------------------------------------------------------
// ORD -- ADR-0009 § 3's ordering obligation, and § 10
// ---------------------------------------------------------------------------

// oneByte is ORD-1's whole request. A single byte cannot be partly consumed, so
// a server that reads anything at all from the connection empties the queue;
// with a longer payload it could consume a prefix and still close on unread
// data.
const oneByte = "7b"

// ORD-1 carries § 3's first three rows and § 10 for a refused peer, through the
// stricter rule the surface freezes: a refused connection is never read. How
// that is observed is sockprobe's, and platform-specific; on Linux the kernel
// reports a close on unread data as ECONNRESET and a close on an empty queue as
// a clean end of stream. The server's own account of its order is not consulted
// -- that is what review of the first version of this case showed cannot be
// trusted.
func TestORD1RefusedConnectionIsNeverRead(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	release := b.armStale("ord1", "-raw-hex", oneByte, "-watch-ms", "10000")
	root := b.session("", "-raw-hex", oneByte, "-watch-ms", "10000")
	b.revoke(stale)
	for who, s := range map[string]session{"stale member": release(), "root": root} {
		if !s.ConsumptionObservable {
			t.Fatalf("%s: this platform has no instrument that can tell a consumed byte from an unconsumed one; ORD-1 cannot be measured here and must not pass", who)
		}
		denied(t, who, s)
		if s.Ended != endedReset {
			t.Fatalf("%s: the connection ended %q after the denial; the server consumed the request byte of a connection it refused (want %q)", who, s.Ended, endedReset)
		}
	}
	b.authorizedControl()
}

// ordRepetitions is how many times ORD-2 kills a server on its denial. A server
// that answers before its record is durable loses the record only when the kill
// lands inside that gap; repetition is what makes the case likely to land there.
const ordRepetitions = 5

func TestORD2DenialRecordDurableBeforeResponse(t *testing.T) {
	b := newBox(t)
	for i := 0; i < ordRepetitions; i++ {
		// A SIGKILLed server leaves its socket behind. Removing it keeps this case
		// independent of stale-socket recovery, which is REC-4's.
		b.sh("rm -f " + socketPath)
		pid := b.start(defaultConfig())
		b.authorizedControl()
		fifo := fmt.Sprintf("/tmp/ord2-%d.fifo", i)
		killed := fmt.Sprintf("/tmp/ord2-%d.kill", i)
		b.sh("mkfifo -m 0666 " + fifo)
		b.detach("", probeBin, "kill-on-fifo", "-fifo", fifo, "-pid", strconv.Itoa(pid), "-out", killed)
		release := b.armStale(fmt.Sprintf("ord2-%d", i), "-frame", request(controlOp, nil), "-watch-ms", "10000", "-notify-fifo", fifo)
		b.revoke(stale)
		denied(t, fmt.Sprintf("repetition %d", i), release())
		for n := 0; n < 100 && b.serverPID() != 0; n++ {
			b.exec("", "sleep", "0.1")
		}
		if out, err := b.exec("", "cat", killed); err != nil || !strings.Contains(out, "killed") {
			t.Fatalf("repetition %d: the server was not killed on its response: %s", i, out)
		}
		if got := len(b.audit().withAction(actionDenied)); got != i+1 {
			t.Fatalf("repetition %d: %d denial records survived a SIGKILL on the response, want %d -- the response was written before its record was durable", i, got, i+1)
		}
		b.grant(stale)
	}
}

func TestORD3NoDenialAnsweredWithoutDurableRecord(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	b.authorizedControl()
	release := b.armStale("ord3", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	// Fill the store's filesystem. The write the denial record needs now fails
	// for a real reason, not an injected one.
	b.sh("dd if=/dev/zero of=" + storeDir + "/fill bs=4096 2>/dev/null; true")
	b.revoke(stale)
	s := release()
	if s.ConnectErrno != "" {
		t.Fatalf("the stale session's connect() failed: %s", s.ConnectErrno)
	}
	if len(s.Frames) != 0 || s.Trailing != 0 {
		t.Fatalf("the server answered a denial it could not record: %+v", s)
	}
	if s.EOFMS < 0 {
		t.Fatalf("the server neither answered nor closed the connection: %+v", s)
	}
	b.sh("rm -f " + storeDir + "/fill")
	if n := len(b.audit().withAction(actionDenied)); n != 0 {
		t.Fatalf("%d denial records exist for a denial that could not be recorded", n)
	}
	// The server survived, and records and answers once it can.
	denied(t, "root after the disk was freed", b.session("", "-frame", request(controlOp, nil), "-watch-ms", "10000"))
	if n := len(b.audit().withAction(actionDenied)); n != 1 {
		t.Fatalf("after the disk was freed a denial produced %d records, want 1", n)
	}
	b.authorizedControl()
}

// ---------------------------------------------------------------------------
// DENY -- ADR-0009 §§ 3, 5, 6, 7
// ---------------------------------------------------------------------------

func TestDENY1StaleMemberDeniedInBand(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	release := b.armStale("deny1", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	b.revoke(stale)
	denied(t, "stale member", release())
	b.authorizedControl()
}

func TestDENY2RootOutsideGroupDeniedInBand(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	s := b.session("", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	if s.UID != 0 || hasGroup(s.Groups, adminGID) {
		t.Fatalf("fixture: the root session is uid %d with groups %v; it must be root outside the admin group", s.UID, s.Groups)
	}
	denied(t, "root", s)
	b.authorizedControl()
}

func TestDENY3DenialSaysOnlyAuthorizationDenied(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	release := b.armStale("deny3", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	b.revoke(stale)
	st := release()
	root := b.session("", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	denied(t, "stale member", st)
	denied(t, "root", root)
	if st.Frames[0] != root.Frames[0] {
		t.Fatalf("the two denials differ, so a caller can tell which check refused it: stale %q, root %q", st.Frames[0], root.Frames[0])
	}
	b.authorizedControl()
}

func TestDENY4InBandDenialIsRecorded(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	b.authorizedControl()
	if n := len(b.audit().withAction(actionDenied)); n != 0 {
		t.Fatalf("an authorized connection produced %d denial records", n)
	}
	release := b.armStale("deny4", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	b.revoke(stale)
	denied(t, "stale member", release())
	if n := len(b.audit().withAction(actionDenied)); n != 1 {
		t.Fatalf("one stale denial produced %d denial records, want 1", n)
	}
	denied(t, "root", b.session("", "-frame", request(controlOp, nil), "-watch-ms", "10000"))
	if n := len(b.audit().withAction(actionDenied)); n != 2 {
		t.Fatalf("a root denial did not add exactly one record: %d total, want 2", n)
	}
}

func TestDENY5KernelRefusalIsNotRecorded(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	b.authorizedControl()
	before := len(b.audit().Rows)
	s := b.session(outsider, "-frame", request(controlOp, nil), "-watch-ms", "2000")
	if s.ConnectErrno != "EACCES" {
		t.Fatalf("fixture: the outsider was not refused by the kernel: %+v", s)
	}
	if after := len(b.audit().Rows); after != before {
		t.Fatalf("a connect() the kernel refused produced %d audit records; the server never observed it", after-before)
	}
}

func TestDENY6RecordActorIsKernelDerived(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	release := b.armStale("deny6", "-frame", request(controlOp, nil), "-watch-ms", "10000")
	b.revoke(stale)
	denied(t, "stale member", release())
	a := b.audit()
	for _, c := range a.Columns {
		if strings.Contains(strings.ToLower(c), forbiddenColumnFragment) {
			t.Errorf("the audit table has column %q; ADR-0009 § 7 records no pid", c)
		}
	}
	rows := a.withAction(actionDenied)
	if len(rows) != 1 {
		t.Fatalf("want one denial record, got %v", rows)
	}
	r := rows[0]
	if got := actorUID(t, r); got != staleUID {
		t.Errorf("%s = %d, want the peer's uid %d", columnActorUID, got, staleUID)
	}
	if got := r[columnActorUsernameSnapshot]; got != stale {
		t.Errorf("%s = %v, want %q", columnActorUsernameSnapshot, got, stale)
	}
	for _, c := range []string{columnJobID, columnTarget} {
		if v, present := r[c]; !present || v != nil {
			t.Errorf("%s = %v (present %v); a denial concerns no job and no agent, so it must be NULL", c, v, present)
		}
	}
}

func TestDENY7RequestCannotSetActor(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	forged := request(controlOp, map[string]any{
		"actor": alice, "actor_uid": aliceUID, "uid": 0, "user": alice, "as": "root",
		columnActorUsernameSnapshot: alice,
	})
	release := b.armStale("deny7", "-frame", forged, "-watch-ms", "10000")
	b.revoke(stale)
	denied(t, "stale member with forged fields", release())
	rows := b.audit().withAction(actionDenied)
	if len(rows) != 1 {
		t.Fatalf("want one denial record, got %v", rows)
	}
	if got := actorUID(t, rows[0]); got != staleUID || rows[0][columnActorUsernameSnapshot] != stale {
		t.Fatalf("the record names %d/%v; the request's fields set the actor, which must be %d/%s", got, rows[0][columnActorUsernameSnapshot], staleUID, stale)
	}
	b.authorizedControl()
}

// ---------------------------------------------------------------------------
// REC -- ADR-0009 § 8. Each refusal is followed by a control that removes the
// planted object and starts the server, so the refusal is shown to be caused by
// that object and not by a server that never starts.
// ---------------------------------------------------------------------------

func (b *box) plantDir(perm string) {
	b.t.Helper()
	b.sh(fmt.Sprintf("mkdir -p %s && chown root:%s %s && chmod %s %s", socketDir, adminGroup, socketDir, perm, socketDir))
}

func (b *box) plantStaleSocket() uint64 {
	b.t.Helper()
	var r map[string]string
	b.probe("", &r, "bind-stale", "-path", socketPath)
	st := b.stat("", socketPath)
	if st.Type != "socket" {
		b.t.Fatalf("fixture: no stale socket planted: %+v", st)
	}
	return st.Inode
}

func TestREC1NonSocketIsNotRemoved(t *testing.T) {
	b := newBox(t)
	b.plantDir("0750")
	b.sh("printf evidence > " + socketPath)
	before := b.stat("", socketPath)
	b.refuse(defaultConfig())
	after := b.stat("", socketPath)
	if after.Type != "file" || after.Inode != before.Inode || b.sh("cat "+socketPath) != "evidence" {
		t.Fatalf("the file at the socket path was changed or removed: before %+v, after %+v", before, after)
	}
	b.sh("rm " + socketPath)
	b.start(defaultConfig())
	b.authorizedControl()
}

func TestREC2ForeignOwnedSocketIsNotRemoved(t *testing.T) {
	b := newBox(t)
	b.plantDir("0750")
	inode := b.plantStaleSocket()
	b.sh(fmt.Sprintf("chown %d %s", aliceUID, socketPath))
	b.refuse(defaultConfig())
	if st := b.stat("", socketPath); st.Type != "socket" || st.Inode != inode || st.UID != aliceUID {
		t.Fatalf("a socket owned by uid %d was removed or replaced: %+v", aliceUID, st)
	}
	b.sh("rm " + socketPath)
	b.start(defaultConfig())
	b.authorizedControl()
}

func TestREC3LiveSocketIsNotRemoved(t *testing.T) {
	b := newBox(t)
	b.plantDir("0750")
	b.detach("", probeBin, "listen", "-path", socketPath)
	var inode uint64
	for i := 0; i < 50; i++ {
		if st := b.stat("", socketPath); st.Type == "socket" {
			inode = st.Inode
			break
		}
		b.exec("", "sleep", "0.1")
	}
	if inode == 0 {
		t.Fatal("fixture: the live listener did not bind")
	}
	b.refuse(defaultConfig())
	if st := b.stat("", socketPath); st.Type != "socket" || st.Inode != inode {
		t.Fatalf("a live socket was removed or replaced: %+v", st)
	}
	if s := b.session("", "-watch-ms", "200"); s.ConnectErrno != "" {
		t.Fatalf("the live server's socket no longer accepts connections: %+v", s)
	}
	// Anchored: an unanchored pattern also matches the shell running pkill,
	// whose own command line contains it.
	b.sh("pkill -f '^" + probeBin + " listen'; rm -f " + socketPath)
	b.start(defaultConfig())
	b.authorizedControl()
}

func TestREC4StaleSocketIsRecovered(t *testing.T) {
	b := newBox(t)
	b.plantDir("0750")
	b.plantStaleSocket()
	if s := b.session("", "-watch-ms", "200"); s.ConnectErrno != "ECONNREFUSED" {
		t.Fatalf("fixture: the planted socket is not stale: %+v", s)
	}
	b.start(defaultConfig())
	// The evidence of replacement is that the path which refused every
	// connection now answers one. An inode comparison cannot show it: tmpfs
	// reuses the number of an unlinked inode, and did so when this was measured.
	b.authorizedControl()
}

func TestREC5WritableDirectoryRefusesStart(t *testing.T) {
	b := newBox(t)
	b.plantDir("0770")
	inode := b.plantStaleSocket()
	b.refuse(defaultConfig())
	if st := b.stat("", socketPath); st.Type != "socket" || st.Inode != inode {
		t.Fatalf("a socket in a directory the admin group can write was removed; its check-then-unlink is not safe there: %+v", st)
	}
	b.sh("chmod 0750 " + socketDir)
	b.start(defaultConfig())
	b.authorizedControl()
}

// ---------------------------------------------------------------------------
// LIM -- ADR-0009 § 9
// ---------------------------------------------------------------------------

func served(s session) bool {
	return s.ConnectErrno == "" && len(s.Frames) == 1 && strings.Contains(s.Frames[0], errUnknownOperation)
}

// closedUnanswered reports a connection the server closed without writing a
// byte.
func closedUnanswered(s session) bool {
	return s.ConnectErrno == "" && len(s.Frames) == 0 && s.Trailing == 0 && s.EOFMS >= 0
}

func TestLIM1UnauthorizedConnectionLimit(t *testing.T) {
	b := newBox(t)
	c := defaultConfig()
	c.MaxUnauthorized, c.HoldBeforeMS, c.AuthTimeoutMS = 2, 2000, 20000
	b.start(c)
	rs := b.multi(alice, "-count", "3", "-frame", request(controlOp, nil), "-watch-ms", "10000", "-until-frames", "1")
	var ok, shed int
	for _, s := range rs {
		switch {
		case served(s) && s.FrameMS[0] >= 1000:
			ok++
		case closedUnanswered(s) && s.EOFMS < 1000:
			shed++
		default:
			t.Errorf("a connection was neither served after the hold nor shed at once: %+v", s)
		}
	}
	if ok != 2 || shed != 1 {
		t.Fatalf("with a limit of 2 awaiting authorization, 3 connections gave %d served and %d shed; want 2 and 1", ok, shed)
	}
}

func TestLIM2AuthorizationTimeoutFreesSlot(t *testing.T) {
	b := newBox(t)
	c := defaultConfig()
	c.MaxUnauthorized, c.AuthTimeoutMS, c.HoldBeforeMS = 1, 300, 5000
	b.start(c)
	// B connects after A's timeout. If A's slot were still held, B would be shed
	// at once (LIM-1); timing out at the same point as A shows the slot was freed.
	rs := b.multi(alice, "-count", "2", "-stagger-ms", "1000", "-frame", request(controlOp, nil), "-watch-ms", "4000")
	for i, s := range rs {
		if !closedUnanswered(s) || s.EOFMS < 200 || s.EOFMS > 2500 {
			t.Fatalf("connection %d was not closed unanswered at the %dms authorization timeout: %+v", i, c.AuthTimeoutMS, s)
		}
	}
	b.stop()
	c.HoldBeforeMS = 0
	b.start(c)
	b.authorizedControl()
}

func TestLIM3AuthorizedConnectionLimit(t *testing.T) {
	b := newBox(t)
	c := defaultConfig()
	c.MaxAuthorized = 1
	b.start(c)
	rs := b.multi(alice, "-count", "2", "-stagger-ms", "500", "-frame", request(controlOp, nil), "-watch-ms", "3000")
	if !served(rs[0]) || rs[0].EOFMS >= 0 {
		t.Fatalf("the first authorized connection was not served and held open: %+v", rs[0])
	}
	if len(rs[1].Frames) != 1 || errorCode(t, rs[1].Frames[0]) != errConnectionLimit || rs[1].EOFMS < 0 {
		t.Fatalf("the second authorized connection was not refused with %s and closed: %+v", errConnectionLimit, rs[1])
	}
	// Both are closed now; the slot must be usable again.
	b.authorizedControl()
}

func TestLIM4OversizedFrameRefusedBeforeAllocation(t *testing.T) {
	b := newBox(t)
	c := defaultConfig()
	c.MaxFrameBytes = 64
	pid := b.start(c)
	b.authorizedControl()
	var before, after struct {
		Error    string `json:"error"`
		VmPeakKB int    `json:"vm_peak_kb"`
	}
	b.probe("", &before, "vm", "-pid", strconv.Itoa(pid))
	// A prefix of 0xfffffff0 -- just under 4 GiB -- and a few body bytes. Go
	// backs a large slice lazily, so a server that allocated the prefix would
	// survive any memory cap; its peak VIRTUAL size is what shows it.
	s := b.session(alice, "-raw-hex", "fffffff0"+strings.Repeat("7b", 32), "-watch-ms", "10000", "-until-frames", "1")
	b.probe("", &after, "vm", "-pid", strconv.Itoa(pid))
	if before.Error != "" || after.Error != "" {
		t.Fatalf("reading the server's VmPeak: %q %q", before.Error, after.Error)
	}
	if len(s.Frames) != 1 || errorCode(t, s.Frames[0]) != errFrameTooLarge {
		t.Fatalf("an oversized prefix was not refused with %s: %+v", errFrameTooLarge, s)
	}
	if grew := after.VmPeakKB - before.VmPeakKB; grew > 256*1024 {
		t.Fatalf("the server's peak virtual size grew by %d KiB on an oversized prefix; it allocated before refusing", grew)
	}
	b.authorizedControl()
}

// ---------------------------------------------------------------------------
// ISO -- ADR-0009 § 11
// ---------------------------------------------------------------------------

func TestISO1NoNetworkSocket(t *testing.T) {
	b := newBox(t)
	pid := b.start(defaultConfig())
	b.authorizedControl()
	var r struct {
		Error string   `json:"error"`
		Inet  []string `json:"inet"`
		Unix  int      `json:"unix"`
	}
	b.probe("", &r, "inet-sockets", "-pid", strconv.Itoa(pid))
	if r.Error != "" {
		t.Fatalf("reading the server's sockets: %s", r.Error)
	}
	if len(r.Inet) != 0 {
		t.Fatalf("the server holds IP sockets %v with no broker configured; nothing may reach an agent except through the broker", r.Inet)
	}
	if r.Unix == 0 {
		t.Fatal("fixture: the server holds no unix socket, so this observation saw nothing")
	}
}

// ---------------------------------------------------------------------------
// FLT -- ADR-0010 § 12
// ---------------------------------------------------------------------------

func TestFLT1EnabledFaultPointIsAudited(t *testing.T) {
	b := newBox(t)
	b.start(defaultConfig())
	if rows := b.audit().withActionPrefix(actionFaultPrefix); len(rows) != 0 {
		t.Fatalf("a server with no fault point enabled recorded %v", rows)
	}
	b.stop()
	c := defaultConfig()
	c.HoldBeforeMS = 1
	b.start(c)
	// Read before any connection: the record must precede accepting one.
	if rows := b.audit().withAction(actionFaultPrefix + keyHoldBeforeDecision); len(rows) != 1 {
		t.Fatalf("a server started with %s enabled has %d records of it, want 1", keyHoldBeforeDecision, len(rows))
	}
	b.authorizedControl()
}
