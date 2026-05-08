package bootstrap

import (
	"context"
	"net"
	"path/filepath"
	"testing"
)

// listenLocal binds an ephemeral TCP port for the validator's
// dial-test to succeed deterministically. Returns "host:port" and a
// stop func.
func listenLocal(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestDefaultValidator_HappyPath(t *testing.T) {
	addr, stop := listenLocal(t)
	defer stop()
	v := NewDefaultValidator(discardLogger())
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(t.TempDir(), "agent.yaml"),
		JoinURL:     "nats://" + addr,
	}
	res, err := v.Validate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.AllOK() {
		t.Errorf("AllOK = false, checks=%+v", res.Checks)
	}
}

func TestDefaultValidator_RejectsBadConfiguration(t *testing.T) {
	v := NewDefaultValidator(discardLogger())
	cfg := &Configuration{
		Mode:       ModeDemo,
		AgentID:    "agent-1",
		ConfigPath: filepath.Join(t.TempDir(), "agent.yaml"),
		// ClusterName missing — Configuration.Validate fails.
	}
	res, err := v.Validate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.AllOK() {
		t.Error("AllOK = true, want false (bad configuration)")
	}
}

func TestDefaultValidator_DialFailureFlagged(t *testing.T) {
	v := NewDefaultValidator(discardLogger())
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(t.TempDir(), "agent.yaml"),
		JoinURL:     "nats://127.0.0.1:1", // assume nothing's listening on TCP/1
	}
	res, err := v.Validate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.AllOK() {
		t.Error("AllOK = true, want false (dial should fail)")
	}
	var foundJoinFail bool
	for _, c := range res.Checks {
		if c.Name == "join_url" && !c.OK {
			foundJoinFail = true
		}
	}
	if !foundJoinFail {
		t.Errorf("expected failed join_url check; got %+v", res.Checks)
	}
}

func TestDefaultValidator_ParentDirAbsentStillOK(t *testing.T) {
	v := NewDefaultValidator(discardLogger())
	// Parent doesn't exist yet — Install will mkdir it.
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(t.TempDir(), "deeply", "nested", "agent.yaml"),
	}
	res, err := v.Validate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !res.AllOK() {
		t.Errorf("AllOK = false, checks=%+v", res.Checks)
	}
}

func TestValidationResult_AllOKEmpty(t *testing.T) {
	r := &ValidationResult{}
	if !r.AllOK() {
		t.Error("AllOK on empty checks should be true")
	}
}
