package controlplane

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
	"github.com/titananvil/titan-anvil/pkg/config"
	natsmgr "github.com/titananvil/titan-anvil/pkg/nats"
	"github.com/titananvil/titan-anvil/pkg/state"
)

func setupTestCommandDispatcher(t *testing.T) (*CommandDispatcher, *ConnectionManager, func()) {
	// Setup NATS
	natsCfg := &config.NATSConfig{
		Mode: config.NATSModeEmbedded,
		Embedded: config.NATSEmbeddedConfig{
			Port:            4222,
			EnableJetStream: true,
		},
	}

	natsM, err := natsmgr.NewManager(natsCfg)
	if err != nil {
		t.Fatalf("Failed to create NATS manager: %v", err)
	}

	if err := natsM.Start(); err != nil {
		t.Fatalf("Failed to start NATS manager: %v", err)
	}

	// Setup connection manager
	cm := NewConnectionManager(natsM)
	if err := cm.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	// Setup state store
	tmpFile := fmt.Sprintf("/tmp/test-dispatcher-%d.db", time.Now().UnixNano())
	storeCfg := &state.Config{
		Backend:    "sqlite",
		SQLitePath: tmpFile,
		SQLiteWAL:  true,
	}

	store, err := state.NewSQLiteStore(storeCfg)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create dispatcher
	dispatcher := NewCommandDispatcher(cm, store)

	cleanup := func() {
		dispatcher.store.Close()
		cm.Stop()
		natsM.Shutdown()
		os.Remove(tmpFile)
	}

	return dispatcher, cm, cleanup
}

func TestNewCommandDispatcher(t *testing.T) {
	dispatcher, _, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	if dispatcher == nil {
		t.Fatal("CommandDispatcher should not be nil")
	}

	if dispatcher.pendingCommands == nil {
		t.Fatal("pendingCommands map should be initialized")
	}

	if dispatcher.commandCallbacks == nil {
		t.Fatal("commandCallbacks map should be initialized")
	}
}

func TestCommandDispatcher_ExecuteCommand(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-1"] = &AgentInfo{
		ID:     "test-agent-1",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-1",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId: "test-agent-1",
		Command: "echo",
		Args:    []string{"hello"},
		Timeout: 300,
	}

	responseChan, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if responseChan == nil {
		t.Fatal("Response channel should not be nil")
	}

	// Verify command was saved to database
	time.Sleep(100 * time.Millisecond)

	if req.CommandId == "" {
		t.Error("Command ID should have been generated")
	}

	cmd, err := dispatcher.GetCommand(ctx, req.CommandId)
	if err != nil {
		t.Fatalf("Failed to get command from database: %v", err)
	}

	if cmd.Command != "echo" {
		t.Errorf("Expected command 'echo', got '%s'", cmd.Command)
	}

	if cmd.Status != pb.CommandStatus_COMMAND_STATUS_RUNNING {
		t.Errorf("Expected status RUNNING, got %v", cmd.Status)
	}

	// Verify command is tracked
	dispatcher.mu.RLock()
	_, exists := dispatcher.pendingCommands[req.CommandId]
	dispatcher.mu.RUnlock()

	if !exists {
		t.Error("Command should be in pending commands map")
	}
}

func TestCommandDispatcher_ExecuteCommand_OfflineAgent(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an offline agent
	cm.mu.Lock()
	cm.agents["offline-agent"] = &AgentInfo{
		ID:     "offline-agent",
		Status: pb.AgentStatus_AGENT_STATUS_OFFLINE,
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId: "offline-agent",
		Command: "echo",
	}

	_, err := dispatcher.ExecuteCommand(ctx, req)
	if err == nil {
		t.Error("Expected error when executing command on offline agent")
	}
}

func TestCommandDispatcher_ExecuteCommand_UnknownAgent(t *testing.T) {
	dispatcher, _, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId: "unknown-agent",
		Command: "echo",
	}

	_, err := dispatcher.ExecuteCommand(ctx, req)
	if err == nil {
		t.Error("Expected error when executing command on unknown agent")
	}
}

func TestCommandDispatcher_HandleCommandResponse_Stdout(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-2"] = &AgentInfo{
		ID:     "test-agent-2",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-2",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId:   "test-agent-2",
		CommandId: "cmd-stdout-1",
		Command:   "echo",
		Args:      []string{"test"},
	}

	responseChan, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Simulate stdout response
	stdoutResp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-stdout-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
		Data:      []byte("test output\n"),
	}

	dispatcher.HandleCommandResponse(stdoutResp)

	// Check if response was sent to channel
	select {
	case resp := <-responseChan:
		if resp.Type != pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT {
			t.Errorf("Expected STDOUT response type, got %v", resp.Type)
		}
		if string(resp.Data) != "test output\n" {
			t.Errorf("Expected data 'test output\\n', got '%s'", string(resp.Data))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestCommandDispatcher_HandleCommandResponse_Completed(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-3"] = &AgentInfo{
		ID:     "test-agent-3",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-3",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId:   "test-agent-3",
		CommandId: "cmd-complete-1",
		Command:   "echo",
	}

	responseChan, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Send some stdout
	stdoutResp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-complete-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
		Data:      []byte("output"),
	}
	dispatcher.HandleCommandResponse(stdoutResp)

	// Drain the stdout response
	<-responseChan

	// Send completion
	completeResp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-complete-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
		ExitCode:  0,
	}
	dispatcher.HandleCommandResponse(completeResp)

	// Check if response was sent to channel
	select {
	case resp := <-responseChan:
		if resp.Type != pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED {
			t.Errorf("Expected COMPLETED response type, got %v", resp.Type)
		}
		if resp.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", resp.ExitCode)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for completion response")
	}

	// Channel should be closed now
	_, ok := <-responseChan
	if ok {
		t.Error("Response channel should be closed after completion")
	}

	// Command should be removed from pending
	dispatcher.mu.RLock()
	_, exists := dispatcher.pendingCommands["cmd-complete-1"]
	dispatcher.mu.RUnlock()

	if exists {
		t.Error("Command should be removed from pending after completion")
	}

	// Verify database was updated
	time.Sleep(100 * time.Millisecond)
	cmd, err := dispatcher.GetCommand(ctx, "cmd-complete-1")
	if err != nil {
		t.Fatalf("Failed to get command from database: %v", err)
	}

	if cmd.Status != pb.CommandStatus_COMMAND_STATUS_COMPLETED {
		t.Errorf("Expected status COMPLETED in database, got %v", cmd.Status)
	}

	if cmd.Stdout != "output" {
		t.Errorf("Expected stdout 'output', got '%s'", cmd.Stdout)
	}
}

func TestCommandDispatcher_HandleCommandResponse_Failed(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-4"] = &AgentInfo{
		ID:     "test-agent-4",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-4",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId:   "test-agent-4",
		CommandId: "cmd-failed-1",
		Command:   "false",
	}

	responseChan, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Send failure
	failResp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-failed-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED,
		ExitCode:  1,
		Error:     "command failed",
	}
	dispatcher.HandleCommandResponse(failResp)

	// Check response
	select {
	case resp := <-responseChan:
		if resp.Type != pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED {
			t.Errorf("Expected FAILED response type, got %v", resp.Type)
		}
		if resp.ExitCode != 1 {
			t.Errorf("Expected exit code 1, got %d", resp.ExitCode)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for failure response")
	}

	// Verify database
	time.Sleep(100 * time.Millisecond)
	cmd, err := dispatcher.GetCommand(ctx, "cmd-failed-1")
	if err != nil {
		t.Fatalf("Failed to get command from database: %v", err)
	}

	if cmd.Status != pb.CommandStatus_COMMAND_STATUS_FAILED {
		t.Errorf("Expected status FAILED in database, got %v", cmd.Status)
	}
}

func TestCommandDispatcher_HandleCommandResponse_Timeout(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-5"] = &AgentInfo{
		ID:     "test-agent-5",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-5",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId:   "test-agent-5",
		CommandId: "cmd-timeout-1",
		Command:   "sleep",
		Args:      []string{"1000"},
		Timeout:   1,
	}

	responseChan, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Send timeout
	timeoutResp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-timeout-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT,
		Error:     "command timed out",
	}
	dispatcher.HandleCommandResponse(timeoutResp)

	// Check response
	select {
	case resp := <-responseChan:
		if resp.Type != pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT {
			t.Errorf("Expected TIMEOUT response type, got %v", resp.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for timeout response")
	}

	// Verify database
	time.Sleep(100 * time.Millisecond)
	cmd, err := dispatcher.GetCommand(ctx, "cmd-timeout-1")
	if err != nil {
		t.Fatalf("Failed to get command from database: %v", err)
	}

	if cmd.Status != pb.CommandStatus_COMMAND_STATUS_TIMEOUT {
		t.Errorf("Expected status TIMEOUT in database, got %v", cmd.Status)
	}
}

func TestCommandDispatcher_MultipleSubscribers(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-6"] = &AgentInfo{
		ID:     "test-agent-6",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-6",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()
	req := &pb.ExecuteCommandRequest{
		AgentId:   "test-agent-6",
		CommandId: "cmd-multi-1",
		Command:   "echo",
	}

	// Create multiple subscribers
	chan1, err := dispatcher.ExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Add second subscriber manually
	chan2 := make(chan *pb.ExecuteCommandResponse, 100)
	dispatcher.mu.Lock()
	dispatcher.commandCallbacks["cmd-multi-1"] = append(dispatcher.commandCallbacks["cmd-multi-1"], chan2)
	dispatcher.mu.Unlock()

	// Send response
	resp := &pb.ExecuteCommandResponse{
		CommandId: "cmd-multi-1",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
		ExitCode:  0,
	}
	dispatcher.HandleCommandResponse(resp)

	// Both channels should receive the response
	timeout := time.After(1 * time.Second)

	select {
	case r := <-chan1:
		if r.CommandId != "cmd-multi-1" {
			t.Errorf("Chan1: Expected command ID 'cmd-multi-1', got '%s'", r.CommandId)
		}
	case <-timeout:
		t.Fatal("Chan1: Timeout waiting for response")
	}

	select {
	case r := <-chan2:
		if r.CommandId != "cmd-multi-1" {
			t.Errorf("Chan2: Expected command ID 'cmd-multi-1', got '%s'", r.CommandId)
		}
	case <-timeout:
		t.Fatal("Chan2: Timeout waiting for response")
	}
}

func TestCommandDispatcher_ListCommands(t *testing.T) {
	dispatcher, cm, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Register an agent
	cm.mu.Lock()
	cm.agents["test-agent-7"] = &AgentInfo{
		ID:     "test-agent-7",
		Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
		Metadata: &pb.AgentMetadata{
			Hostname: "test-host-7",
		},
		LastHeartbeat: time.Now(),
	}
	cm.mu.Unlock()

	ctx := context.Background()

	// Execute multiple commands
	for i := 1; i <= 3; i++ {
		req := &pb.ExecuteCommandRequest{
			AgentId:   "test-agent-7",
			CommandId: fmt.Sprintf("cmd-list-%d", i),
			Command:   "echo",
			Args:      []string{fmt.Sprintf("test-%d", i)},
		}
		dispatcher.ExecuteCommand(ctx, req)
	}

	time.Sleep(200 * time.Millisecond)

	// List all commands
	commands, err := dispatcher.ListCommands(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list commands: %v", err)
	}

	if len(commands) != 3 {
		t.Errorf("Expected 3 commands, got %d", len(commands))
	}

	// List with filter
	filter := &state.CommandFilter{
		Limit: 2,
	}
	commands, err = dispatcher.ListCommands(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list commands with filter: %v", err)
	}

	if len(commands) != 2 {
		t.Errorf("Expected 2 commands with limit, got %d", len(commands))
	}
}

func TestCommandDispatcher_HandleCommandResponse_UnknownCommand(t *testing.T) {
	dispatcher, _, cleanup := setupTestCommandDispatcher(t)
	defer cleanup()

	// Send response for unknown command (should not crash)
	resp := &pb.ExecuteCommandResponse{
		CommandId: "unknown-cmd",
		Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
	}

	// Should not panic
	dispatcher.HandleCommandResponse(resp)

	// Command should not be in pending
	dispatcher.mu.RLock()
	_, exists := dispatcher.pendingCommands["unknown-cmd"]
	dispatcher.mu.RUnlock()

	if exists {
		t.Error("Unknown command should not be in pending")
	}
}
