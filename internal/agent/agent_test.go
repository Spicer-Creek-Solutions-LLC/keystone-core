package agent

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/config"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"google.golang.org/protobuf/proto"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Helper function to create a test NATS manager with embedded mode
func createTestNATSManager(t *testing.T) *natsmgr.Manager {
	t.Helper()

	// Use a dynamic port to avoid conflicts with parallel tests
	port := helpers.FreePort(t)

	mgr, err := natsmgr.NewManager(&config.NATSConfig{
		Mode: config.NATSModeEmbedded,
		Embedded: config.NATSEmbeddedConfig{
			Port:            port,
			EnableJetStream: false,
			MaxConnections:  100,
		},
		MaxReconnects: 5,
		ReconnectWait: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to create NATS manager: %v", err)
	}

	if err := mgr.Start(); err != nil {
		t.Fatalf("Failed to start NATS manager: %v", err)
	}

	// Wait for connection
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		conn := mgr.Conn()
		return conn != nil && conn.Status() == nats.CONNECTED, nil
	}); err != nil {
		t.Fatalf("Timed out waiting for NATS connection: %v", err)
	}

	return mgr
}

func TestNewAgent(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	t.Run("with provided ID", func(t *testing.T) {
		agent, err := NewAgent("test-agent-1", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
		if err != nil {
			t.Fatalf("NewAgent failed: %v", err)
		}

		if agent.ID() != "test-agent-1" {
			t.Errorf("Expected ID 'test-agent-1', got %s", agent.ID())
		}

		if agent.heartbeatInterval != 30*time.Second {
			t.Errorf("Expected heartbeat interval 30s, got %v", agent.heartbeatInterval)
		}
	})

	t.Run("with auto-generated ID", func(t *testing.T) {
		agent, err := NewAgent("", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
		if err != nil {
			t.Fatalf("NewAgent failed: %v", err)
		}

		if agent.ID() == "" {
			t.Error("Expected auto-generated ID, got empty string")
		}

		// UUID should be 36 characters
		if len(agent.ID()) != 36 {
			t.Errorf("Expected UUID length 36, got %d", len(agent.ID()))
		}
	})
}

func TestAgent_ID(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-id", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	if agent.ID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got %s", agent.ID())
	}
}

func TestAgent_Register(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	t.Run("no control plane response", func(t *testing.T) {
		// Register should succeed even without control plane
		err := agent.register()
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Agent should be marked as registered
		agent.mu.RLock()
		registered := agent.registered
		agent.mu.RUnlock()

		if !registered {
			t.Error("Agent should be marked as registered")
		}
	})

	t.Run("with control plane response", func(t *testing.T) {
		// Create a second agent for this test
		agent2, err := NewAgent("test-agent-2", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
		if err != nil {
			t.Fatalf("NewAgent failed: %v", err)
		}

		// Mock control plane responder
		_, err = mgr.Subscribe("kscore.default.agent.register", func(msg *nats.Msg) {
			var req pb.RegisterRequest
			if err := proto.Unmarshal(msg.Data, &req); err != nil {
				t.Errorf("Failed to unmarshal register request: %v", err)
				return
			}

			// Send response
			resp := &pb.RegisterResponse{
				AgentId: req.AgentId,
				Config: &pb.AgentConfig{
					HeartbeatInterval: 60,
					CommandTimeout:    45,
				},
			}

			data, _ := proto.Marshal(resp)
			msg.Respond(data)
		})
		if err != nil {
			t.Fatalf("Failed to subscribe to register: %v", err)
		}

		if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
			t.Fatalf("Failed to flush register subscription: %v", err)
		}

		// Register
		err = agent2.register()
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Configuration should be updated
		if agent2.heartbeatInterval != 60*time.Second {
			t.Errorf("Expected heartbeat interval 60s, got %v", agent2.heartbeatInterval)
		}

		if agent2.commandTimeout != 45*time.Second {
			t.Errorf("Expected command timeout 45s, got %v", agent2.commandTimeout)
		}
	})
}

func TestAgent_SendHeartbeat(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Subscribe to heartbeats
	heartbeatReceived := make(chan bool, 1)
	_, err = mgr.Subscribe("kscore.default.agent.heartbeat", func(msg *nats.Msg) {
		var req pb.HeartbeatRequest
		if err := proto.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("Failed to unmarshal heartbeat: %v", err)
			return
		}

		if req.AgentId != "test-agent" {
			t.Errorf("Expected agent ID 'test-agent', got %s", req.AgentId)
		}

		if req.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("Expected status ONLINE, got %v", req.Status)
		}

		heartbeatReceived <- true
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to heartbeat: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush heartbeat subscription: %v", err)
	}

	// Send heartbeat
	err = agent.sendHeartbeat()
	if err != nil {
		t.Fatalf("sendHeartbeat failed: %v", err)
	}

	// Wait for heartbeat
	select {
	case <-heartbeatReceived:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Heartbeat not received")
	}
}

func TestAgent_UpdateMetadata(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Record initial metadata time
	agent.mu.RLock()
	initialTime := agent.lastMetadata
	agent.mu.RUnlock()

	// Update metadata
	agent.updateMetadata()

	// Check that lastMetadata was updated
	agent.mu.RLock()
	updatedTime := agent.lastMetadata
	agent.mu.RUnlock()

	if !updatedTime.After(initialTime) {
		t.Error("lastMetadata should be updated")
	}
}

func TestAgent_SubscribeToCommands(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	err = agent.subscribeToCommands()
	if err != nil {
		t.Fatalf("subscribeToCommands failed: %v", err)
	}

	// Verify subscription by sending a command
	subject := fmt.Sprintf("kscore.default.agent.%s.command", agent.ID())

	// Create a simple command request
	req := &pb.ExecuteCommandRequest{
		CommandId: "test-cmd-1",
		Command:   "echo",
		Args:      []string{"hello"},
		Timeout:   5,
	}

	data, _ := proto.Marshal(req)

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush command subscription: %v", err)
	}

	// Publish command (don't wait for response, just verify no error)
	err = mgr.Publish(subject, data)
	if err != nil {
		t.Fatalf("Failed to publish command: %v", err)
	}
}

func TestAgent_HandleCommandRequest(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Subscribe to commands first
	err = agent.subscribeToCommands()
	if err != nil {
		t.Fatalf("subscribeToCommands failed: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush command subscription: %v", err)
	}

	// Create a command request
	req := &pb.ExecuteCommandRequest{
		CommandId: "test-cmd-2",
		Command:   "echo",
		Args:      []string{"test"},
		Timeout:   5,
	}

	data, _ := proto.Marshal(req)

	// Send command and wait for response
	subject := fmt.Sprintf("kscore.default.agent.%s.command", agent.ID())

	// Create a channel to receive responses
	responseChan := make(chan *pb.ExecuteCommandResponse, 5)

	// Subscribe to responses using a unique inbox
	inbox := nats.NewInbox()
	_, err = mgr.Subscribe(inbox, func(msg *nats.Msg) {
		var resp pb.ExecuteCommandResponse
		if err := proto.Unmarshal(msg.Data, &resp); err != nil {
			t.Logf("Failed to unmarshal response: %v", err)
			return
		}
		responseChan <- &resp
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to responses: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush response subscription: %v", err)
	}

	// Create a NATS message with reply subject
	natsMsg := &nats.Msg{
		Subject: subject,
		Reply:   inbox,
		Data:    data,
	}

	// Publish the message
	nc := mgr.Conn()
	if nc == nil {
		t.Fatal("NATS connection is nil")
	}
	err = nc.PublishMsg(natsMsg)
	if err != nil {
		t.Fatalf("Failed to publish command: %v", err)
	}

	// Wait for responses
	completionReceived := false
	timeout := time.After(5 * time.Second)

	for !completionReceived {
		select {
		case resp := <-responseChan:
			if resp.CommandId != "test-cmd-2" {
				t.Errorf("Expected command ID 'test-cmd-2', got %s", resp.CommandId)
			}

			switch resp.Type {
			case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
				pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR:
				// Output received
			case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED:
				completionReceived = true
			case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED:
				t.Errorf("Command failed: %s", resp.Error)
				completionReceived = true
			}

		case <-timeout:
			t.Fatal("Timeout waiting for command completion")
		}
	}
}

func TestAgent_SendCommandResponse(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Create a channel to receive the response
	inbox := nats.NewInbox()
	responseChan := make(chan []byte, 1)

	_, err = mgr.Subscribe(inbox, func(msg *nats.Msg) {
		responseChan <- msg.Data
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush response subscription: %v", err)
	}

	// Create a fake original message with reply
	originalMsg := &nats.Msg{
		Reply: inbox,
	}

	// Create and send response
	resp := &pb.ExecuteCommandResponse{
		CommandId: "test-resp",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
		ExitCode:  0,
	}

	agent.sendCommandResponse(originalMsg, resp)

	// Wait for response
	select {
	case data := <-responseChan:
		var received pb.ExecuteCommandResponse
		if err := proto.Unmarshal(data, &received); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if received.CommandId != "test-resp" {
			t.Errorf("Expected command ID 'test-resp', got %s", received.CommandId)
		}

	case <-time.After(2 * time.Second):
		t.Error("Response not received")
	}
}

func TestAgent_SendCommandError(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Create a channel to receive the error
	inbox := nats.NewInbox()
	errorChan := make(chan []byte, 1)

	_, err = mgr.Subscribe(inbox, func(msg *nats.Msg) {
		errorChan <- msg.Data
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush error subscription: %v", err)
	}

	// Create a fake original message with reply
	originalMsg := &nats.Msg{
		Reply: inbox,
	}

	// Send error
	agent.sendCommandError(originalMsg, "test error", fmt.Errorf("details"))

	// Wait for error
	select {
	case data := <-errorChan:
		if len(data) == 0 {
			t.Error("Expected error data")
		}
	case <-time.After(2 * time.Second):
		t.Error("Error not received")
	}
}

func TestAgent_StartStop(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 100*time.Millisecond, 200*time.Millisecond, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Start agent
	err = agent.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	startTime := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		agent.mu.RLock()
		lastMetadata := agent.lastMetadata
		agent.mu.RUnlock()
		return !lastMetadata.IsZero() && lastMetadata.After(startTime), nil
	}); err != nil {
		t.Fatalf("Agent loops did not update metadata: %v", err)
	}

	// Stop agent
	err = agent.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestAgent_HeartbeatLoop(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	// Create agent with very short heartbeat interval
	agent, err := NewAgent("test-agent", mgr, 50*time.Millisecond, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Count heartbeats
	heartbeatCount := 0
	heartbeatChan := make(chan bool, 10)

	_, err = mgr.Subscribe("kscore.default.agent.heartbeat", func(msg *nats.Msg) {
		heartbeatChan <- true
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush heartbeat subscription: %v", err)
	}

	// Start the heartbeat loop
	agent.wg.Add(1)
	go agent.heartbeatLoop()

	// Collect heartbeats for enough time to get at least 2 (50ms interval + buffer)
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-heartbeatChan:
			heartbeatCount++
		case <-timeout:
			goto done
		}
	}

done:
	// Stop the agent
	agent.cancel()
	agent.wg.Wait()

	// Should have received multiple heartbeats
	if heartbeatCount < 2 {
		t.Errorf("Expected at least 2 heartbeats, got %d", heartbeatCount)
	}
}

func TestAgent_MetadataUpdateLoop(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	// Create agent with very short metadata update interval
	agent, err := NewAgent("test-agent", mgr, 5*time.Minute, 50*time.Millisecond, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Record initial metadata time
	agent.mu.RLock()
	initialTime := agent.lastMetadata
	agent.mu.RUnlock()

	// Start the metadata update loop
	agent.wg.Add(1)
	go agent.metadataUpdateLoop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		agent.mu.RLock()
		lastMetadata := agent.lastMetadata
		agent.mu.RUnlock()
		return lastMetadata.After(initialTime), nil
	}); err != nil {
		t.Fatalf("Timed out waiting for metadata update: %v", err)
	}

	// Stop the agent
	agent.cancel()
	agent.wg.Wait()

	// Check that metadata was updated
	agent.mu.RLock()
	updatedTime := agent.lastMetadata
	agent.mu.RUnlock()

	if !updatedTime.After(initialTime) {
		t.Error("Metadata should have been updated")
	}
}

func TestAgent_ContextCancellation(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 100*time.Millisecond, 100*time.Millisecond, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Start loops
	agent.wg.Add(2)
	go agent.heartbeatLoop()
	go agent.metadataUpdateLoop()

	// Cancel context
	agent.cancel()

	// Wait for goroutines to exit (with timeout)
	done := make(chan bool)
	go func() {
		agent.wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success - goroutines exited
	case <-time.After(2 * time.Second):
		t.Error("Goroutines did not exit after context cancellation")
	}
}

func TestAgent_SendCommandResponse_NoReply(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Create a message without reply subject
	originalMsg := &nats.Msg{
		Reply: "", // No reply subject
	}

	// Create response
	resp := &pb.ExecuteCommandResponse{
		CommandId: "test-no-reply",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
	}

	// Should not panic or error when there's no reply subject
	agent.sendCommandResponse(originalMsg, resp)
}

func TestAgent_HandleCommandRequest_InvalidProto(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Subscribe to commands
	err = agent.subscribeToCommands()
	if err != nil {
		t.Fatalf("subscribeToCommands failed: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush command subscription: %v", err)
	}

	// Send invalid protobuf data
	subject := fmt.Sprintf("kscore.default.agent.%s.command", agent.ID())
	invalidData := []byte("not a valid protobuf message")

	// Create a channel to receive error response
	inbox := nats.NewInbox()
	errorChan := make(chan bool, 1)

	_, err = mgr.Subscribe(inbox, func(msg *nats.Msg) {
		errorChan <- true
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to errors: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush error subscription: %v", err)
	}

	// Send invalid command
	natsMsg := &nats.Msg{
		Subject: subject,
		Reply:   inbox,
		Data:    invalidData,
	}

	nc := mgr.Conn()
	if nc == nil {
		t.Fatal("NATS connection is nil")
	}
	err = nc.PublishMsg(natsMsg)
	if err != nil {
		t.Fatalf("Failed to publish invalid command: %v", err)
	}

	// Wait for error response
	select {
	case <-errorChan:
		// Error was sent successfully
	case <-time.After(2 * time.Second):
		t.Error("Expected error response for invalid proto")
	}
}

func TestExecutor_CancelRunningCommand(t *testing.T) {
	executor := NewExecutor()

	// Start a long-running command
	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "timeout"
		args = []string{"/t", "30", "/nobreak"}
	} else {
		command = "sleep"
		args = []string{"30"}
	}

	req := &ExecuteCommandRequest{
		CommandID: "cancel-test",
		Command:   command,
		Args:      args,
		Timeout:   60 * time.Second,
	}

	// Start execution in background
	resultChan := make(chan *CommandResult, 1)
	go func() {
		ctx := context.Background()
		result, _ := executor.Execute(ctx, req, nil)
		resultChan <- result
	}()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		executor.mu.RLock()
		_, running := executor.runningCommands["cancel-test"]
		executor.mu.RUnlock()
		return running, nil
	}); err != nil {
		t.Fatalf("Timed out waiting for command start: %v", err)
	}

	// Cancel the command
	err := executor.CancelCommand("cancel-test")
	if err != nil {
		t.Fatalf("Failed to cancel command: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.ExitCode == 0 {
			t.Error("Expected non-zero exit code for cancelled command")
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for cancelled command result")
	}
}

func TestExecutor_ContextCancellation(t *testing.T) {
	executor := NewExecutor()

	// Start a long-running command with cancellable context
	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "timeout"
		args = []string{"/t", "30", "/nobreak"}
	} else {
		command = "sleep"
		args = []string{"30"}
	}

	req := &ExecuteCommandRequest{
		CommandID: "context-cancel-test",
		Command:   command,
		Args:      args,
		Timeout:   60 * time.Second,
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start execution in background
	resultChan := make(chan *CommandResult, 1)
	go func() {
		result, _ := executor.Execute(ctx, req, nil)
		resultChan <- result
	}()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		executor.mu.RLock()
		_, running := executor.runningCommands["context-cancel-test"]
		executor.mu.RUnlock()
		return running, nil
	}); err != nil {
		t.Fatalf("Timed out waiting for command start: %v", err)
	}

	// Cancel the context
	cancel()

	// Wait for result
	select {
	case result := <-resultChan:
		if result.ExitCode == 0 {
			t.Error("Expected non-zero exit code for cancelled command")
		}
	case <-time.After(5 * time.Second):
		t.Error("Timeout waiting for context-cancelled command result")
	}
}

func TestExecutor_MultilineOutput(t *testing.T) {
	executor := NewExecutor()

	// Use a command that produces multiple lines of output
	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "cmd"
		args = []string{"/c", "echo", "line1", "&&", "echo", "line2", "&&", "echo", "line3"}
	} else {
		command = "sh"
		args = []string{"-c", "echo line1 && echo line2 && echo line3"}
	}

	outputLines := 0
	handler := func(commandID string, isStderr bool, data []byte) {
		outputLines++
	}

	req := &ExecuteCommandRequest{
		CommandID: "multiline-test",
		Command:   command,
		Args:      args,
		Timeout:   5 * time.Second,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, req, handler)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Should have received multiple output lines
	if outputLines == 0 {
		t.Error("Expected output lines to be captured")
	}
}

func TestAgent_HandleCommandRequest_CommandFailure(t *testing.T) {
	mgr := createTestNATSManager(t)
	defer mgr.Shutdown()

	agent, err := NewAgent("test-agent", mgr, 30*time.Second, 5*time.Minute, 30*time.Second)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}

	// Subscribe to commands
	err = agent.subscribeToCommands()
	if err != nil {
		t.Fatalf("subscribeToCommands failed: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush command subscription: %v", err)
	}

	// Create a command that will fail
	req := &pb.ExecuteCommandRequest{
		CommandId: "failing-cmd",
		Command:   "false", // 'false' command always exits with 1
		Args:      []string{},
		Timeout:   5,
	}

	data, _ := proto.Marshal(req)

	// Send command and wait for response
	subject := fmt.Sprintf("kscore.default.agent.%s.command", agent.ID())

	// Create a channel to receive responses
	responseChan := make(chan *pb.ExecuteCommandResponse, 5)

	// Subscribe to responses using a unique inbox
	inbox := nats.NewInbox()
	_, err = mgr.Subscribe(inbox, func(msg *nats.Msg) {
		var resp pb.ExecuteCommandResponse
		if err := proto.Unmarshal(msg.Data, &resp); err != nil {
			return
		}
		responseChan <- &resp
	})
	if err != nil {
		t.Fatalf("Failed to subscribe to responses: %v", err)
	}

	if err := mgr.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush response subscription: %v", err)
	}

	// Create a NATS message with reply subject
	natsMsg := &nats.Msg{
		Subject: subject,
		Reply:   inbox,
		Data:    data,
	}

	// Publish the message
	nc := mgr.Conn()
	if nc == nil {
		t.Fatal("NATS connection is nil")
	}
	err = nc.PublishMsg(natsMsg)
	if err != nil {
		t.Fatalf("Failed to publish command: %v", err)
	}

	// Wait for completion response
	completionReceived := false
	timeout := time.After(5 * time.Second)

	for !completionReceived {
		select {
		case resp := <-responseChan:
			if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED {
				completionReceived = true
				// Exit code should be non-zero for 'false' command
				if resp.ExitCode == 0 {
					t.Error("Expected non-zero exit code for 'false' command")
				}
			} else if resp.Type == pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED {
				completionReceived = true
			}

		case <-timeout:
			t.Fatal("Timeout waiting for command completion")
		}
	}
}
