//go:build linux

package operator

import (
	"net"

	"golang.org/x/sys/unix"
)

type peerIdentity struct{ Uid uint32 }

func peerCredentials(conn *net.UnixConn) (*peerIdentity, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		cred, controlErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	if controlErr != nil {
		return nil, controlErr
	}
	return &peerIdentity{Uid: cred.Uid}, nil
}
