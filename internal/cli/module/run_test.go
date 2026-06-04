// SPDX-License-Identifier: Apache-2.0

package module

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/verify"
)

// writeRunModule writes a minimal capability-using module (kv + log)
// into dir and returns the directory. main(input) exercises the kv and
// log capability builtins and echoes an input field.
func writeRunModule(t *testing.T, dir string) {
	t.Helper()
	manifestYAML := `name: test/runmod
version: 0.1.0
type: starlark
entrypoint: main.star
capabilities:
  kv: {}
  log: {}
`
	mainStar := `def main(input):
    kv_set("k", "v")
    val, ok = kv_get("k")
    log("info", "module ran")
    return {"echo": input.get("msg", "none"), "kv": val, "ok": ok}
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifestYAML), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.star"), []byte(mainStar), 0o600); err != nil {
		t.Fatalf("write main.star: %v", err)
	}
}

// runCLI invokes the kscore-module command with separate stdout/stderr
// buffers (module log output goes to stderr; the JSON result to stdout).
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := NewCommand(Deps{})
	var so, se bytes.Buffer
	cmd.SetOut(&so)
	cmd.SetErr(&se)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err = cmd.Execute()
	return so.String(), se.String(), err
}

func TestRun_SkipVerification_ExecutesCapabilities(t *testing.T) {
	dir := t.TempDir()
	writeRunModule(t, dir)

	stdout, _, err := runCLI(t, "run", dir, `{"msg":"hello"}`, "--skip-verification")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var out map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &out); jerr != nil {
		t.Fatalf("output not JSON: %v\n%s", jerr, stdout)
	}
	if out["echo"] != "hello" {
		t.Errorf("echo = %v, want hello", out["echo"])
	}
	if out["kv"] != "v" {
		t.Errorf("kv = %v, want v (kv_set/kv_get capability)", out["kv"])
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
}

func TestRun_UnsignedWithoutFlag_Errors(t *testing.T) {
	dir := t.TempDir()
	writeRunModule(t, dir)

	_, _, err := runCLI(t, "run", dir)
	if err == nil {
		t.Fatal("run of an unsigned module without --skip-verification succeeded; want error")
	}
}

func TestRun_SignedModule_VerifiesAndExecutes(t *testing.T) {
	dir := t.TempDir()
	writeRunModule(t, dir)

	// Build the bundle zip, then sign exactly those bytes (run on a zip
	// file uses it as-is, so the signed bytes match what the loader
	// hashes).
	zipPath := filepath.Join(t.TempDir(), "runmod.zip")
	if err := zipDir(dir, zipPath); err != nil {
		t.Fatalf("zipDir: %v", err)
	}
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	sig, err := verify.Sign(zipBytes, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigBytes, err := verify.MarshalSignature(sig)
	if err != nil {
		t.Fatalf("marshal sig: %v", err)
	}
	sigFile := filepath.Join(t.TempDir(), "runmod.sig")
	if werr := os.WriteFile(sigFile, sigBytes, 0o600); werr != nil {
		t.Fatalf("write sig: %v", werr)
	}

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal pubkey: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "signer.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if werr := os.WriteFile(keyFile, keyPEM, 0o600); werr != nil {
		t.Fatalf("write key: %v", werr)
	}

	stdout, _, err := runCLI(t, "run", zipPath, `{"msg":"signed"}`, "--sig", sigFile, "--key", keyFile)
	if err != nil {
		t.Fatalf("run signed: %v", err)
	}
	var out map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &out); jerr != nil {
		t.Fatalf("output not JSON: %v\n%s", jerr, stdout)
	}
	if out["echo"] != "signed" {
		t.Errorf("echo = %v, want signed", out["echo"])
	}

	// A wrong key (different signer) must fail verification.
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	otherDER, _ := x509.MarshalPKIXPublicKey(otherPub)
	otherKey := filepath.Join(t.TempDir(), "other.pem")
	_ = os.WriteFile(otherKey, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: otherDER}), 0o600)
	if _, _, e := runCLI(t, "run", zipPath, "--sig", sigFile, "--key", otherKey); e == nil {
		t.Fatal("run verified against an untrusted key; want error")
	}
}
