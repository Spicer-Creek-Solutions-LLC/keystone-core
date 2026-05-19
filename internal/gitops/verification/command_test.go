package verification

import (
	"context"
	"errors"
	"testing"
)

type fakeRunner struct {
	res    CommandResult
	err    error
	gotReq CommandRequest
}

func (f *fakeRunner) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	f.gotReq = req
	return f.res, f.err
}

func TestCommandVerifier(t *testing.T) {
	t.Parallel()

	t.Run("success default exit 0", func(t *testing.T) {
		t.Parallel()
		fr := &fakeRunner{res: CommandResult{ExitCode: 0, Stdout: "ok"}}
		r := CommandVerifier{Runner: fr}.Verify(context.Background(), Step{
			Config: map[string]any{"command": "true", "args": []any{"-x"}, "env": map[string]any{"K": "V"}},
		})
		if !r.Success {
			t.Fatalf("Success = false: %q %v", r.Message, r.Error)
		}
		if fr.gotReq.Command != "true" || len(fr.gotReq.Args) != 1 || fr.gotReq.Env["K"] != "V" {
			t.Errorf("runner got unexpected req: %+v", fr.gotReq)
		}
		if r.Data["exit_code"] != 0 {
			t.Errorf("Data exit_code = %v", r.Data["exit_code"])
		}
	})

	t.Run("exit code mismatch fails", func(t *testing.T) {
		t.Parallel()
		r := CommandVerifier{Runner: &fakeRunner{res: CommandResult{ExitCode: 1, Stderr: "bad"}}}.
			Verify(context.Background(), Step{Config: map[string]any{"command": "false"}})
		if r.Success {
			t.Fatal("Success = true, want false")
		}
		if r.Data["exit_code"] != 1 {
			t.Errorf("Data exit_code = %v, want 1", r.Data["exit_code"])
		}
	})

	t.Run("expect_exit_code honoured", func(t *testing.T) {
		t.Parallel()
		r := CommandVerifier{Runner: &fakeRunner{res: CommandResult{ExitCode: 3}}}.
			Verify(context.Background(), Step{Config: map[string]any{"command": "x", "expect_exit_code": 3}})
		if !r.Success {
			t.Errorf("Success = false, want true (3 == expected 3)")
		}
	})

	t.Run("launch error", func(t *testing.T) {
		t.Parallel()
		r := CommandVerifier{Runner: &fakeRunner{err: errors.New("no such file")}}.
			Verify(context.Background(), Step{Config: map[string]any{"command": "ghost"}})
		if r.Success || r.Error == nil {
			t.Errorf("want failed result with error, got %+v", r)
		}
	})

	t.Run("nil runner fails", func(t *testing.T) {
		t.Parallel()
		r := CommandVerifier{}.Verify(context.Background(), Step{Config: map[string]any{"command": "x"}})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("nil runner: want ErrConfig failure, got %+v", r)
		}
	})

	t.Run("missing command is config error", func(t *testing.T) {
		t.Parallel()
		r := CommandVerifier{Runner: &fakeRunner{}}.Verify(context.Background(), Step{Config: map[string]any{}})
		if r.Success || !errors.Is(r.Error, ErrConfig) {
			t.Errorf("want ErrConfig, got %+v", r)
		}
	})
}

func TestCommandVerifier_Type(t *testing.T) {
	t.Parallel()
	if (CommandVerifier{}).Type() != "command" {
		t.Error("Type() != command")
	}
}

func TestSnippet(t *testing.T) {
	t.Parallel()
	if got := snippet("short"); got != "short" {
		t.Errorf("snippet(short) = %q", got)
	}
	big := make([]byte, maxCmdSnippet+10)
	if got := snippet(string(big)); len(got) != maxCmdSnippet {
		t.Errorf("snippet len = %d, want %d", len(got), maxCmdSnippet)
	}
}
