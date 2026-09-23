//go:build !linux

package operator

import (
	"errors"
	"net"
)

type peerIdentity struct{ Uid uint32 }

func peerCredentials(*net.UnixConn) (*peerIdentity, error) {
	return nil, errors.New("kernel peer credentials are not implemented on this platform")
}
