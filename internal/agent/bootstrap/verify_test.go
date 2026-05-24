// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDefaultVerifier_HappyPath(t *testing.T) {
	addr, stop := listenLocal(t)
	defer stop()

	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		JoinURL:     "nats://" + addr,
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
	}
	// Install first, otherwise the verifier finds no config file.
	if _, err := NewDefaultInstaller(discardLogger()).Install(context.Background(), cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}

	res, err := NewDefaultVerifier(discardLogger()).Verify(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.AllOK() {
		t.Errorf("AllOK = false, checks=%+v", res.Checks)
	}
}

func TestDefaultVerifier_MissingConfigFails(t *testing.T) {
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(t.TempDir(), "agent.yaml"),
	}
	res, err := NewDefaultVerifier(discardLogger()).Verify(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.AllOK() {
		t.Error("AllOK = true, want false (config file missing)")
	}
}

func TestDefaultVerifier_DialFailureFlagged(t *testing.T) {
	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
		JoinURL:     "nats://127.0.0.1:1",
	}
	if _, err := NewDefaultInstaller(discardLogger()).Install(context.Background(), cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	res, err := NewDefaultVerifier(discardLogger()).Verify(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.AllOK() {
		t.Error("AllOK = true, want false (dial should fail)")
	}
}

func TestVerifyResult_AllOKEmpty(t *testing.T) {
	r := &VerifyResult{}
	if !r.AllOK() {
		t.Error("AllOK on empty checks should be true")
	}
}
