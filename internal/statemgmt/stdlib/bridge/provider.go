package bridge

import (
	"context"
	"errors"
)

var ErrUnsupportedOS = errors.New("bridge: unsupported OS for v0.1 (Linux only)")
var ErrNoIP = errors.New("bridge: the ip (iproute2) binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIP(err error) bool          { return errors.Is(err, ErrNoIP) }

type LinkInfo struct {
	Name string
	Kind string
}

type BridgeSpec struct {
	Name string
	STP  bool
}

type Provider interface {
	GetLink(ctx context.Context, name string) (*LinkInfo, error)
	CreateBridge(ctx context.Context, s BridgeSpec) error
	DeleteLink(ctx context.Context, name string) error
	SetMaster(ctx context.Context, child, master string) error
}

type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
