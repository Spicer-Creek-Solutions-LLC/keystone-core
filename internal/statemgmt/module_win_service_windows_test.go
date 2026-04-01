// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type fakeService struct {
	states []svc.State
	idx    int
	err    error
}

func (f *fakeService) Query() (svc.Status, error) {
	if f.err != nil {
		return svc.Status{}, f.err
	}
	if len(f.states) == 0 {
		return svc.Status{State: svc.Stopped}, nil
	}
	if f.idx >= len(f.states) {
		return svc.Status{State: f.states[len(f.states)-1]}, nil
	}
	state := f.states[f.idx]
	f.idx++
	return svc.Status{State: state}, nil
}

func TestWinServiceModule_waitForState_Success(t *testing.T) {
	m := &WinServiceModule{}
	service := &fakeService{states: []svc.State{svc.Running}}

	if err := m.waitForState(service, svc.Running, 100*time.Millisecond); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWinServiceModule_waitForState_Timeout(t *testing.T) {
	m := &WinServiceModule{}
	service := &fakeService{states: []svc.State{svc.Stopped}}

	if err := m.waitForState(service, svc.Running, 50*time.Millisecond); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestWinServiceModule_waitForState_QueryError(t *testing.T) {
	m := &WinServiceModule{}
	expectedErr := errors.New("query failed")
	service := &fakeService{err: expectedErr}

	if err := m.waitForState(service, svc.Running, 50*time.Millisecond); !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
