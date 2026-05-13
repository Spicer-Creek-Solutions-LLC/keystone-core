package vlan

import (
	"context"
	"errors"
)

var ErrUnsupportedOS = errors.New("vlan: unsupported OS for v0.1 (Linux only)")
var ErrNoIP = errors.New("vlan: the ip (iproute2) binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoIP(err error) bool          { return errors.Is(err, ErrNoIP) }

type LinkInfo struct {
	Name string
	Kind string
}

type VLANSpec struct {
	Name   string
	Parent string
	ID     int
}

type Provider interface {
	GetLink(ctx context.Context, name string) (*LinkInfo, error)
	CreateVLAN(ctx context.Context, s VLANSpec) error
	DeleteLink(ctx context.Context, name string) error
}

type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
