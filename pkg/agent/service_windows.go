// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
)

const (
	// ServiceName is the Windows service name
	ServiceName = "kscore-agent"

	// ServiceDisplayName is the display name in services.msc
	ServiceDisplayName = "Keystone Core Agent"

	// ServiceDescription is the service description
	ServiceDescription = "Keystone Core agent for infrastructure management"

	// DefaultShutdownTimeout is the default graceful shutdown timeout
	DefaultShutdownTimeout = 30 * time.Second
)

// WindowsService wraps an Agent to run as a Windows service
type WindowsService struct {
	agent           *Agent
	shutdownTimeout time.Duration
	mu              sync.Mutex
	running         bool
	stopChan        chan struct{}
}

// NewWindowsService creates a new WindowsService wrapper
func NewWindowsService(agent *Agent) *WindowsService {
	return &WindowsService{
		agent:           agent,
		shutdownTimeout: DefaultShutdownTimeout,
		stopChan:        make(chan struct{}),
	}
}

// SetShutdownTimeout sets the graceful shutdown timeout
func (s *WindowsService) SetShutdownTimeout(timeout time.Duration) {
	s.shutdownTimeout = timeout
}

// Execute implements svc.Handler interface
// This is called by the Windows Service Control Manager
func (s *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	// Accept stop, shutdown, pause, and continue commands
	const acceptedCmds = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	// Report that we're starting
	changes <- svc.Status{State: svc.StartPending}

	// Start the agent
	if err := s.agent.Start(); err != nil {
		// Report failure and exit
		changes <- svc.Status{State: svc.Stopped}
		return true, 1
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// Report that we're running
	changes <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

	// Main service loop
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				// Report current status
				changes <- c.CurrentStatus

			case svc.Stop, svc.Shutdown:
				// Begin graceful shutdown
				changes <- svc.Status{State: svc.StopPending}

				// Create context with timeout for graceful shutdown
				ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)

				// Stop the agent
				stopErr := s.stopAgent(ctx)
				cancel()

				if stopErr != nil {
					// Log error but continue with shutdown
					// In production, this would go to Event Log
				}

				s.mu.Lock()
				s.running = false
				s.mu.Unlock()

				changes <- svc.Status{State: svc.Stopped}
				return false, 0

			case svc.Pause:
				// Pause the agent (stop heartbeats but stay connected)
				changes <- svc.Status{State: svc.Paused, Accepts: acceptedCmds}

			case svc.Continue:
				// Resume the agent
				changes <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

			default:
				// Unknown command, ignore
			}

		case <-s.stopChan:
			// External stop request
			changes <- svc.Status{State: svc.StopPending}

			ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
			s.stopAgent(ctx)
			cancel()

			s.mu.Lock()
			s.running = false
			s.mu.Unlock()

			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}

// stopAgent stops the agent with context
func (s *WindowsService) stopAgent(ctx context.Context) error {
	done := make(chan error, 1)

	go func() {
		done <- s.agent.Stop()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop signals the service to stop
func (s *WindowsService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		close(s.stopChan)
	}
}

// IsRunning returns whether the service is running
func (s *WindowsService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// RunAsService runs the agent as a Windows service
// This should be called from main() when running as a service
func RunAsService(agent *Agent) error {
	ws := NewWindowsService(agent)
	return svc.Run(ServiceName, ws)
}

// IsWindowsService checks if the process is running as a Windows service
func IsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// ServiceStatus represents the status of the Windows service
type ServiceStatus struct {
	State       string
	ProcessID   uint32
	ExitCode    uint32
	Checkpoint  uint32
	WaitHint    uint32
	Accepts     uint32
	IsRunning   bool
	IsPaused    bool
	IsStopped   bool
	IsPending   bool
	Description string
}

// stateToString converts svc.State to a human-readable string
func stateToString(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "Start Pending"
	case svc.StopPending:
		return "Stop Pending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "Continue Pending"
	case svc.PausePending:
		return "Pause Pending"
	case svc.Paused:
		return "Paused"
	default:
		return fmt.Sprintf("Unknown (%d)", state)
	}
}

// StateDescription returns a description of the given state
func StateDescription(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "The service is not running"
	case svc.StartPending:
		return "The service is starting"
	case svc.StopPending:
		return "The service is stopping"
	case svc.Running:
		return "The service is running"
	case svc.ContinuePending:
		return "The service is resuming"
	case svc.PausePending:
		return "The service is pausing"
	case svc.Paused:
		return "The service is paused"
	default:
		return "Unknown state"
	}
}
