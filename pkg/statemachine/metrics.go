package statemachine

import "sync/atomic"

// MachineMetrics tracks error and panic counts for a state machine.
type MachineMetrics struct {
	transitionErrors      atomic.Int64
	invalidTransitions    atomic.Int64
	guardFailures         atomic.Int64
	machineClosed         atomic.Int64
	concurrentTransitions atomic.Int64
	callbackPanics        atomic.Int64
}

// MachineMetricsSnapshot is a point-in-time view of metrics.
type MachineMetricsSnapshot struct {
	TransitionErrors      int64
	InvalidTransitions    int64
	GuardFailures         int64
	MachineClosed         int64
	ConcurrentTransitions int64
	CallbackPanics        int64
}

func (m *MachineMetrics) Snapshot() MachineMetricsSnapshot {
	if m == nil {
		return MachineMetricsSnapshot{}
	}
	return MachineMetricsSnapshot{
		TransitionErrors:      m.transitionErrors.Load(),
		InvalidTransitions:    m.invalidTransitions.Load(),
		GuardFailures:         m.guardFailures.Load(),
		MachineClosed:         m.machineClosed.Load(),
		ConcurrentTransitions: m.concurrentTransitions.Load(),
		CallbackPanics:        m.callbackPanics.Load(),
	}
}

func (m *MachineMetrics) recordInvalidTransition() {
	if m == nil {
		return
	}
	m.transitionErrors.Add(1)
	m.invalidTransitions.Add(1)
}

func (m *MachineMetrics) recordGuardFailure() {
	if m == nil {
		return
	}
	m.transitionErrors.Add(1)
	m.guardFailures.Add(1)
}

func (m *MachineMetrics) recordMachineClosed() {
	if m == nil {
		return
	}
	m.transitionErrors.Add(1)
	m.machineClosed.Add(1)
}

func (m *MachineMetrics) recordConcurrentTransition() {
	if m == nil {
		return
	}
	m.transitionErrors.Add(1)
	m.concurrentTransitions.Add(1)
}

func (m *MachineMetrics) recordCallbackPanic() {
	if m == nil {
		return
	}
	m.callbackPanics.Add(1)
}
