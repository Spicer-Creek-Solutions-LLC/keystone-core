// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package agent

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

func TestWaitForServiceState_Success(t *testing.T) {
	service := &fakeService{states: []svc.State{svc.Stopped}}

	if err := waitForServiceState(service, svc.Stopped, 100*time.Millisecond); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWaitForServiceState_Timeout(t *testing.T) {
	service := &fakeService{states: []svc.State{svc.Running}}

	if err := waitForServiceState(service, svc.Stopped, 50*time.Millisecond); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestWaitForServiceState_QueryError(t *testing.T) {
	expectedErr := errors.New("query failed")
	service := &fakeService{err: expectedErr}

	if err := waitForServiceState(service, svc.Stopped, 50*time.Millisecond); !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
