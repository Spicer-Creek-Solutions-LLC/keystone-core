package cluster

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeLeadership struct {
	mu     sync.Mutex
	leader bool
	obs    []LeadershipObserver
}

func (f *fakeLeadership) AddObserver(o LeadershipObserver) {
	f.mu.Lock()
	f.obs = append(f.obs, o)
	f.mu.Unlock()
}
func (f *fakeLeadership) RemoveObserver(o LeadershipObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.obs {
		if x == o {
			f.obs = append(f.obs[:i], f.obs[i+1:]...)
			return
		}
	}
}
func (f *fakeLeadership) IsLeader() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leader
}
func (f *fakeLeadership) set(b bool) {
	f.mu.Lock()
	f.leader = b
	f.mu.Unlock()
}

type fakeTask struct {
	name     string
	mu       sync.Mutex
	starts   int
	stops    int
	startErr error
}

func (t *fakeTask) Name() string { return t.name }
func (t *fakeTask) Start(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts++
	return t.startErr
}
func (t *fakeTask) Stop(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stops++
	return nil
}
func (t *fakeTask) snap() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.starts, t.stops
}

func elected(self bool) LeadershipEvent {
	if self {
		return LeadershipEvent{State: LeaderElected, Self: true}
	}
	return LeadershipEvent{State: LeaderCampaigning, Self: false}
}

func newSTM(t *testing.T, ls leadershipSource, tasks ...SingletonTask) *SingletonTaskManager {
	t.Helper()
	m, err := NewSingletonTaskManager(SingletonTaskManagerConfig{Leadership: ls})
	if err != nil {
		t.Fatalf("NewSingletonTaskManager: %v", err)
	}
	for _, tk := range tasks {
		if err := m.Register(tk); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background()) })
	return m
}

func TestNewSingletonTaskManager_InvalidConfig(t *testing.T) {
	if _, err := NewSingletonTaskManager(SingletonTaskManagerConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}

func TestSingleton_StartStopOnLeadershipTransitions(t *testing.T) {
	ls := &fakeLeadership{} // not leader
	a, b := &fakeTask{name: "a"}, &fakeTask{name: "b"}
	m := newSTM(t, ls, a, b)

	// Not leader at Start → nothing running.
	if s, _ := a.snap(); s != 0 {
		t.Fatalf("task a started before leadership: %d", s)
	}
	if m.IsRunning() {
		t.Fatal("manager running before leadership")
	}

	// Gain leadership → all tasks Started once.
	m.OnLeadershipChange(elected(true))
	for _, tk := range []*fakeTask{a, b} {
		if s, _ := tk.snap(); s != 1 {
			t.Fatalf("%s starts = %d, want 1", tk.name, s)
		}
	}
	// Repeated Elected → no double-start.
	m.OnLeadershipChange(elected(true))
	if s, _ := a.snap(); s != 1 {
		t.Fatalf("double-start on repeated Elected: a starts = %d", s)
	}

	// Lose leadership → all Stopped.
	m.OnLeadershipChange(elected(false))
	for _, tk := range []*fakeTask{a, b} {
		if s, st := tk.snap(); s != 1 || st != 1 {
			t.Fatalf("%s = (%d,%d), want (1,1)", tk.name, s, st)
		}
	}

	// Flap back → Start again.
	m.OnLeadershipChange(elected(true))
	if s, _ := a.snap(); s != 2 {
		t.Fatalf("a starts after flap = %d, want 2", s)
	}
}

func TestSingleton_StartWhenAlreadyLeader(t *testing.T) {
	ls := &fakeLeadership{leader: true}
	a := &fakeTask{name: "a"}
	m := newSTM(t, ls, a)
	if s, _ := a.snap(); s != 1 {
		t.Fatalf("task not started on Start-while-leader: %d", s)
	}
	if !m.IsRunning() {
		t.Fatal("manager not running while leader")
	}
}

func TestSingleton_TaskStartErrorDoesNotBlockOthers(t *testing.T) {
	ls := &fakeLeadership{}
	bad := &fakeTask{name: "bad", startErr: errors.New("boom")}
	good := &fakeTask{name: "good"}
	m := newSTM(t, ls, bad, good)

	m.OnLeadershipChange(elected(true))
	if s, _ := good.snap(); s != 1 {
		t.Fatalf("good task not started despite bad task error: %d", s)
	}
	if !m.IsRunning() {
		t.Fatal("manager should still consider itself leader-running")
	}
}

func TestSingleton_RegisterAfterStartRejected(t *testing.T) {
	ls := &fakeLeadership{}
	m := newSTM(t, ls)
	if err := m.Register(&fakeTask{name: "late"}); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("Register after Start = %v, want ErrAlreadyStarted", err)
	}
}

func TestSingleton_LifecycleErrors(t *testing.T) {
	ls := &fakeLeadership{}
	m, err := NewSingletonTaskManager(SingletonTaskManagerConfig{Leadership: ls})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := m.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v", err)
	}
}

func TestSingleton_LeaderCheckPassthrough(t *testing.T) {
	ls := &fakeLeadership{}
	m := newSTM(t, ls)
	lc := m.LeaderCheck()
	if lc() {
		t.Fatal("LeaderCheck true while not leader")
	}
	ls.set(true)
	if !lc() {
		t.Fatal("LeaderCheck false after becoming leader")
	}
}

func TestSingleton_StopStopsRunningTasks(t *testing.T) {
	ls := &fakeLeadership{leader: true}
	a := &fakeTask{name: "a"}
	m, _ := NewSingletonTaskManager(SingletonTaskManagerConfig{Leadership: ls})
	_ = m.Register(a)
	_ = m.Start(context.Background())
	if s, _ := a.snap(); s != 1 {
		t.Fatalf("task not started: %d", s)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, st := a.snap(); st != 1 {
		t.Fatalf("running task not stopped on manager Stop: stops=%d", st)
	}
}

func TestLoopTask_RunsWhileLeaderThenStops(t *testing.T) {
	var calls atomic.Int64
	lt := LoopTask("tick", 20*time.Millisecond, func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if lt.Name() != "tick" {
		t.Fatalf("Name = %q", lt.Name())
	}
	if err := lt.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := lt.Start(context.Background()); err != nil {
		t.Fatalf("idempotent Start: %v", err)
	}
	waitFor(t, func() bool { return calls.Load() >= 3 }, "loop task to tick")
	if err := lt.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	after := calls.Load()
	time.Sleep(120 * time.Millisecond)
	if calls.Load() != after {
		t.Fatalf("loop task kept running after Stop: %d → %d", after, calls.Load())
	}
	if err := lt.Stop(context.Background()); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestStartStopTask_Adapter(t *testing.T) {
	var started, stopped atomic.Bool
	tk := StartStopTask("x",
		func(context.Context) error { started.Store(true); return nil },
		func(context.Context) error { stopped.Store(true); return nil },
	)
	if tk.Name() != "x" {
		t.Fatalf("Name = %q", tk.Name())
	}
	_ = tk.Start(context.Background())
	_ = tk.Stop(context.Background())
	if !started.Load() || !stopped.Load() {
		t.Fatalf("adapter did not delegate: started=%v stopped=%v", started.Load(), stopped.Load())
	}
	// nil funcs are no-ops.
	nilTk := StartStopTask("n", nil, nil)
	if err := nilTk.Start(context.Background()); err != nil {
		t.Fatalf("nil start = %v", err)
	}
	if err := nilTk.Stop(context.Background()); err != nil {
		t.Fatalf("nil stop = %v", err)
	}
}

func TestSingleton_IntegrationWithRealLeaderElector(t *testing.T) {
	ec, _ := newEmbedded(t)
	le := startElector(t, ec, "node-a") // becomes leader

	task := &fakeTask{name: "leader-only"}
	m, err := NewSingletonTaskManager(SingletonTaskManagerConfig{Leadership: le})
	if err != nil {
		t.Fatalf("new STM: %v", err)
	}
	if err := m.Register(task); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("STM Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background()) })

	// Either the Start initial-sync or the LeadershipObserver
	// Elected callback starts the task once leadership is held.
	waitFor(t, func() bool { s, _ := task.snap(); return s >= 1 }, "leader-only task to start under real election")
	if !m.IsRunning() {
		t.Fatal("manager not running though node is leader")
	}
}
