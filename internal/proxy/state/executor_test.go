package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/proxy"
)

type fakeExecutor struct {
	exitCodes map[string]int
	stdout    map[string]string
}

func (f *fakeExecutor) Execute(ctx context.Context, req *proxy.ProxiedExecuteRequest) (*proxy.ProxiedExecuteResult, error) {
	code := f.exitCodes[req.Command]
	out := f.stdout[req.Command]
	return &proxy.ProxiedExecuteResult{
		DeviceID:  req.DeviceID,
		ExitCode:  code,
		Stdout:    []byte(out),
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}, nil
}

func (f *fakeExecutor) ExecuteWithOutput(ctx context.Context, req *proxy.ProxiedExecuteRequest, outputHandler proxy.OutputHandler) (*proxy.ProxiedExecuteResult, error) {
	result, err := f.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	if outputHandler != nil && len(result.Stdout) > 0 {
		outputHandler(req.DeviceID, false, result.Stdout)
	}
	return result, nil
}

func (f *fakeExecutor) Check(ctx context.Context, deviceID string) (*proxy.DeviceHealthResult, error) {
	return &proxy.DeviceHealthResult{DeviceID: deviceID, Status: proxy.DeviceStatusOnline}, nil
}

func (f *fakeExecutor) GetCapabilities(ctx context.Context, deviceID string) (*proxy.DeviceCapabilities, error) {
	return &proxy.DeviceCapabilities{DeviceID: deviceID, CanExecuteCommands: true}, nil
}

func (f *fakeExecutor) Close(ctx context.Context, deviceID string) error {
	return nil
}

type fakeModule struct {
	mu            sync.Mutex
	name          string
	executeErr    error
	executeResult *ModuleResult
	calls         int
	lastDryRun    bool
}

func (m *fakeModule) Name() string {
	return m.name
}

func (m *fakeModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	m.mu.Lock()
	m.calls++
	m.lastDryRun = mctx.DryRun
	m.mu.Unlock()
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	if m.executeResult != nil {
		return m.executeResult, nil
	}
	return &ModuleResult{Changed: false, Comment: "ok"}, nil
}

func (m *fakeModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	return &ModuleResult{Changed: false, Comment: "check"}, nil
}

type captureEmitter struct {
	events []Event
}

func (c *captureEmitter) Emit(event Event) {
	c.events = append(c.events, event)
}

func newTestManager(t *testing.T, deviceID string) *proxy.Manager {
	t.Helper()
	cfg := proxy.DefaultManagerConfig()
	cfg.AgentID = "proxy-1"
	manager, err := proxy.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	device := &proxy.ProxiedDevice{
		ID:           deviceID,
		ProxyAgentID: cfg.AgentID,
		Name:         "device",
		Type:         proxy.DeviceTypeNetwork,
		Protocol:     proxy.ProtocolSSH,
		Address:      "127.0.0.1",
		ProfileID:    "profile-1",
		Status:       proxy.DeviceStatusOnline,
	}
	if err := manager.Registry().Register(context.Background(), device); err != nil {
		t.Fatalf("Register device failed: %v", err)
	}
	return manager
}

func TestNewProxyStateExecutor_NilManager(t *testing.T) {
	if _, err := NewProxyStateExecutor(nil, DefaultExecutorConfig()); err == nil {
		t.Fatal("Expected error for nil manager")
	}
}

func TestProxyStateExecutor_Execute_DeviceNotFound(t *testing.T) {
	cfg := proxy.DefaultManagerConfig()
	cfg.AgentID = "proxy-1"
	manager, err := proxy.NewManager(cfg)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	run, err := executor.Execute(context.Background(), "missing", nil)
	if err == nil {
		t.Fatal("Expected error for missing device")
	}
	if run.Error == nil {
		t.Fatal("Expected run error to be set")
	}
}

func TestProxyStateExecutor_Execute_NoExecutor(t *testing.T) {
	manager := newTestManager(t, "device-1")
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	run, err := executor.Execute(context.Background(), "device-1", nil)
	if err == nil {
		t.Fatal("Expected error for missing executor")
	}
	if run.Error == nil {
		t.Fatal("Expected run error to be set")
	}
}

func TestProxyStateExecutor_Execute_UnknownModule(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	run, err := executor.Execute(context.Background(), "device-1", []ProxyStateDeclaration{
		{ID: "one", Name: "one", Module: "missing"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(run.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(run.Results))
	}
	if run.Results[0].Result != ResultFailed {
		t.Fatalf("Expected failed result, got %s", run.Results[0].Result)
	}
}

func TestProxyStateExecutor_Execute_SkippedCondition(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{
		exitCodes: map[string]int{"false": 1},
	})
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	module := &fakeModule{name: "test_module"}
	executor.modules.Register("test_module", module)

	run, err := executor.Execute(context.Background(), "device-1", []ProxyStateDeclaration{
		{
			ID:         "one",
			Name:       "one",
			Module:     "test_module",
			Conditions: ProxyConditions{OnlyIf: "false"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if module.calls != 0 {
		t.Fatalf("Expected module not to execute, got %d calls", module.calls)
	}
	if run.Results[0].Result != ResultSkipped {
		t.Fatalf("Expected skipped result, got %s", run.Results[0].Result)
	}
}

func TestProxyStateExecutor_Execute_ChangedAndEvents(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	module := &fakeModule{
		name: "test_module",
		executeResult: &ModuleResult{
			Changed: true,
			Comment: "updated",
		},
	}
	executor.modules.Register("test_module", module)

	events := &captureEmitter{}
	executor.SetEventEmitter(events)

	run, err := executor.Execute(context.Background(), "device-1", []ProxyStateDeclaration{
		{ID: "one", Name: "one", Module: "test_module"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if run.Summary.Changed != 1 {
		t.Fatalf("Expected 1 changed state, got %d", run.Summary.Changed)
	}
	if len(events.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events.events))
	}
}

func TestProxyStateExecutor_Check_SetsDryRun(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	config := DefaultExecutorConfig()
	config.DryRun = false
	executor, err := NewProxyStateExecutor(manager, config)
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	module := &fakeModule{name: "test_module"}
	executor.modules.Register("test_module", module)

	_, err = executor.Check(context.Background(), "device-1", []ProxyStateDeclaration{
		{ID: "one", Name: "one", Module: "test_module"},
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !module.lastDryRun {
		t.Fatal("Expected DryRun to be true during check")
	}
	if executor.config.DryRun {
		t.Fatal("Expected DryRun to be restored after check")
	}
}

func TestProxyStateExecutor_OrderStates(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	states := []ProxyStateDeclaration{
		{ID: "a", Name: "a", Module: "test"},
		{ID: "b", Name: "b", Module: "test", Requisites: []ProxyRequisite{{Type: "require", State: "a"}}},
	}
	ordered, err := executor.orderStates(states)
	if err != nil {
		t.Fatalf("orderStates failed: %v", err)
	}
	if len(ordered) != 2 || ordered[0].ID != "b" {
		t.Fatalf("Unexpected order: %+v", ordered)
	}

	_, err = executor.orderStates([]ProxyStateDeclaration{
		{ID: "a", Module: "test", Requisites: []ProxyRequisite{{Type: "require", State: "b"}}},
		{ID: "b", Module: "test", Requisites: []ProxyRequisite{{Type: "require", State: "a"}}},
	})
	if err == nil {
		t.Fatal("Expected cycle error")
	}
}

func TestProxyStateExecutor_ExecuteBatch(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	if err := manager.Registry().Register(context.Background(), &proxy.ProxiedDevice{
		ID:           "device-2",
		ProxyAgentID: "proxy-1",
		Name:         "device-2",
		Type:         proxy.DeviceTypeNetwork,
		Protocol:     proxy.ProtocolSSH,
		Address:      "127.0.0.2",
		ProfileID:    "profile-2",
		Status:       proxy.DeviceStatusOnline,
	}); err != nil {
		t.Fatalf("Register device-2 failed: %v", err)
	}

	executor, err := NewProxyStateExecutor(manager, DefaultExecutorConfig())
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	module := &fakeModule{name: "test_module"}
	executor.modules.Register("test_module", module)

	results, err := executor.ExecuteBatch(context.Background(), []string{"device-1", "device-2"}, []ProxyStateDeclaration{
		{ID: "one", Name: "one", Module: "test_module"},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
}

func TestProxyStateExecutor_FailFast(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})
	config := DefaultExecutorConfig()
	config.FailFast = true
	executor, err := NewProxyStateExecutor(manager, config)
	if err != nil {
		t.Fatalf("NewProxyStateExecutor failed: %v", err)
	}

	module := &fakeModule{
		name:       "test_module",
		executeErr: errors.New("boom"),
	}
	executor.modules.Register("test_module", module)

	run, err := executor.Execute(context.Background(), "device-1", []ProxyStateDeclaration{
		{ID: "one", Name: "one", Module: "test_module"},
		{ID: "two", Name: "two", Module: "test_module"},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if run.Error == nil {
		t.Fatal("Expected run error for fail-fast execution")
	}
	if len(run.Results) != 1 {
		t.Fatalf("Expected fail-fast to stop after 1 result, got %d", len(run.Results))
	}
}
