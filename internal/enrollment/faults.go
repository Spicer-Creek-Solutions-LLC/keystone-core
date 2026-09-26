package enrollment

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

// Fault events, as the frozen crash harness names them.
const (
	FaultEnabled = "enabled"
	FaultReached = "reached"
)

// FaultRecord is one line of the agent's fault journal (ADR-0010 § 5). A
// harness waits for the durable "reached" line before it kills the agent; the
// line proves only where the agent stopped, never that anything was in order.
type FaultRecord struct {
	Version int    `json:"version"`
	Event   string `json:"event"`
	Fault   string `json:"fault"`
	Process string `json:"process"`
}

type faults struct {
	cfg config.AgentFaults
}

// record appends one line and fsyncs it before returning, so a record the
// harness can read is one that survives the kill it is waiting to make.
func (f faults) record(event, fault string) error {
	b, err := json.Marshal(FaultRecord{Version: 1, Event: event, Fault: fault, Process: "agent"})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(f.cfg.RecordPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, ArtifactMode)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// enabled records every enabled hold at startup: a fault point in the shipped
// binary is acceptable because its enabling is on record.
func (f faults) enabled() error {
	for _, h := range f.cfg.AgentHolds() {
		if h.Hold > 0 {
			if err := f.record(FaultEnabled, h.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

// hold stops at a boundary, if its hold is enabled: the durable reached record
// first, then the wait.
func (f faults) hold(ctx context.Context, key string) error {
	for _, h := range f.cfg.AgentHolds() {
		if h.Key != key || h.Hold <= 0 {
			continue
		}
		if err := f.record(FaultReached, key); err != nil {
			return err
		}
		select {
		case <-time.After(h.Hold):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
