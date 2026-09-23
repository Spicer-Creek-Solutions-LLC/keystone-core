//go:build linux

package sockprobe

// ConsumptionObservable reports whether, on this platform, a client can tell
// from outside the server whether the server consumed the bytes it was sent.
//
// On Linux it can. When a unix stream socket is closed while its receive queue
// still holds data, the kernel marks the peer ECONNRESET (af_unix's
// unix_release_sock); when the queue is empty, the peer reads a clean end of
// stream. Data the server wrote before closing is delivered first either way.
// So a client that sends ONE byte and is then closed on learns exactly one
// thing: EndedReset means the byte was never consumed, EndedEOF means it was.
// One byte, because a longer payload lets a server consume a prefix and still
// leave data queued.
//
// It cannot see MSG_PEEK, which inspects without consuming.
// unread_linux_test.go asserts all of this on the running kernel, so a kernel
// that changed the behaviour fails there rather than passing every case.
//
// This is the platform seam C04-A's evidence describes. The requirement the
// contract freezes is platform-neutral; a port supplies this function and its
// kernel test for its own platform, and a platform with no such observation
// returns false, which fails the ordering case instead of passing it.
func ConsumptionObservable() bool { return true }
