package gnmi

import (
	"context"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

// Reboot requests a device reboot via gNOI System service.
func (a *Adapter) Reboot(_ context.Context, _ protocols.GNMIRebootMethod, _ string) error {
	return fmt.Errorf("gNOI Reboot not supported: requires openconfig/gnoi dependency")
}

// Ping executes a network ping via gNOI System service.
func (a *Adapter) Ping(_ context.Context, _ string, _ int32, _ int64) ([]*protocols.GNMIPingResponse, error) {
	return nil, fmt.Errorf("gNOI Ping not supported: requires openconfig/gnoi dependency")
}

// Traceroute executes a traceroute via gNOI System service.
func (a *Adapter) Traceroute(_ context.Context, _ string, _ int32) ([]*protocols.GNMITracerouteResponse, error) {
	return nil, fmt.Errorf("gNOI Traceroute not supported: requires openconfig/gnoi dependency")
}
