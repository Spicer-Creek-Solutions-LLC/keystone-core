package policy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func biPolicy(id, code string) *policy.Policy {
	return &policy.Policy{
		ID:              id,
		Name:            id,
		Type:            audit.PolicyTypeBuiltin,
		Category:        policy.CategorySecurity,
		Severity:        audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit,
		Code:            code,
		Enabled:         true,
	}
}

func mustEval(t *testing.T, e *policy.BuiltinEvaluator, code string, in policy.EvaluationInput) policy.EvaluationResult {
	t.Helper()
	res, err := e.Evaluate(context.Background(), biPolicy("p", code), in)
	if err != nil {
		t.Fatalf("Evaluate(%s): unexpected error %v", code, err)
	}
	return res
}

func TestBuiltin_RuleTable(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()

	tests := []struct {
		name      string
		code      string
		in        policy.EvaluationInput
		wantAllow bool
	}{
		// require-labels
		{"require-labels pass", `{"rule":"require-labels","keys":["owner","env"]}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"owner": "team-a", "env": "prod"}}}, true},
		{"require-labels missing", `{"rule":"require-labels","keys":["owner","env"]}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"owner": "team-a"}}}, false},
		{"require-labels empty value", `{"rule":"require-labels","keys":["owner"]}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"owner": "  "}}}, false},
		{"require-labels no labels map", `{"rule":"require-labels","keys":["owner"]}`,
			policy.EvaluationInput{Resource: map[string]any{}}, false},

		// require-owner
		{"require-owner default field pass", `{"rule":"require-owner"}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"owner": "alice"}}}, true},
		{"require-owner custom field deny", `{"rule":"require-owner","field":"team"}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"owner": "alice"}}}, false},

		// allowed-environments
		{"allowed-environments pass", `{"rule":"allowed-environments","allowed":["dev","prod"]}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"env": "prod"}}}, true},
		{"allowed-environments deny", `{"rule":"allowed-environments","allowed":["dev"]}`,
			policy.EvaluationInput{Resource: map[string]any{"labels": map[string]any{"env": "prod"}}}, false},

		// allowed-actions
		{"allowed-actions pass", `{"rule":"allowed-actions","allowed":["read","list"]}`,
			policy.EvaluationInput{Action: "read"}, true},
		{"allowed-actions deny", `{"rule":"allowed-actions","allowed":["read"]}`,
			policy.EvaluationInput{Action: "delete"}, false},

		// deny-privileged
		{"deny-privileged not set pass", `{"rule":"deny-privileged"}`,
			policy.EvaluationInput{Resource: map[string]any{}}, true},
		{"deny-privileged bool true deny", `{"rule":"deny-privileged"}`,
			policy.EvaluationInput{Resource: map[string]any{"privileged": true}}, false},
		{"deny-privileged string yes deny", `{"rule":"deny-privileged","field":"priv"}`,
			policy.EvaluationInput{Resource: map[string]any{"priv": "yes"}}, false},
		{"deny-privileged string false pass", `{"rule":"deny-privileged"}`,
			policy.EvaluationInput{Resource: map[string]any{"privileged": "false"}}, true},

		// allowed-users
		{"allowed-users pass", `{"rule":"allowed-users","allowed":["alice","bob"]}`,
			policy.EvaluationInput{User: "alice"}, true},
		{"allowed-users deny", `{"rule":"allowed-users","allowed":["alice"]}`,
			policy.EvaluationInput{User: "mallory"}, false},

		// no-root-execution
		{"no-root pass", `{"rule":"no-root-execution"}`,
			policy.EvaluationInput{User: "alice"}, true},
		{"no-root deny user root", `{"rule":"no-root-execution"}`,
			policy.EvaluationInput{User: "root"}, false},
		{"no-root deny uid 0 field", `{"rule":"no-root-execution","field":"uid"}`,
			policy.EvaluationInput{Resource: map[string]any{"uid": "0"}}, false},

		// require-approval
		{"require-approval pass", `{"rule":"require-approval"}`,
			policy.EvaluationInput{Context: map[string]any{"approved": true}}, true},
		{"require-approval missing deny", `{"rule":"require-approval"}`,
			policy.EvaluationInput{Context: map[string]any{}}, false},
		{"require-approval string 1 pass", `{"rule":"require-approval","field":"ok"}`,
			policy.EvaluationInput{Context: map[string]any{"ok": "1"}}, true},

		// max-concurrent
		{"max-concurrent pass", `{"rule":"max-concurrent","max":5}`,
			policy.EvaluationInput{Context: map[string]any{"concurrent": float64(3)}}, true},
		{"max-concurrent exceed deny", `{"rule":"max-concurrent","max":5}`,
			policy.EvaluationInput{Context: map[string]any{"concurrent": float64(9)}}, false},
		{"max-concurrent non-numeric deny", `{"rule":"max-concurrent","max":5}`,
			policy.EvaluationInput{Context: map[string]any{"concurrent": "lots"}}, false},

		// resource-quota
		{"resource-quota pass", `{"rule":"resource-quota","field":"cpu","max":100}`,
			policy.EvaluationInput{Resource: map[string]any{"cpu": float64(80)}}, true},
		{"resource-quota exceed deny", `{"rule":"resource-quota","field":"cpu","max":100}`,
			policy.EvaluationInput{Resource: map[string]any{"cpu": float64(128)}}, false},

		// pattern-allow
		{"pattern-allow match pass", `{"rule":"pattern-allow","field":"image","patterns":["registry.internal/*"]}`,
			policy.EvaluationInput{Resource: map[string]any{"image": "registry.internal/app:1.2"}}, true},
		{"pattern-allow no match deny", `{"rule":"pattern-allow","field":"image","patterns":["registry.internal/*"]}`,
			policy.EvaluationInput{Resource: map[string]any{"image": "docker.io/evil"}}, false},

		// pattern-deny
		{"pattern-deny no match pass", `{"rule":"pattern-deny","field":"image","patterns":["*:latest"]}`,
			policy.EvaluationInput{Resource: map[string]any{"image": "app:1.2"}}, true},
		{"pattern-deny match deny", `{"rule":"pattern-deny","field":"image","patterns":["*:latest"]}`,
			policy.EvaluationInput{Resource: map[string]any{"image": "app:latest"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := mustEval(t, e, tt.code, tt.in)
			if res.Allowed != tt.wantAllow {
				t.Errorf("Allowed = %v, want %v (violations: %+v)", res.Allowed, tt.wantAllow, res.Violations)
			}
			if !res.Allowed && len(res.Violations) == 0 {
				t.Errorf("deny with no violations")
			}
			if !res.Allowed && res.Violations[0].Severity != audit.SeverityHigh {
				t.Errorf("violation severity = %v, want policy severity High", res.Violations[0].Severity)
			}
			if res.PolicyID != "p" || res.EvaluatedAt.IsZero() {
				t.Errorf("result envelope wrong: %+v", res)
			}
		})
	}
}

func TestBuiltin_TimeWindow(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()
	code := `{"rule":"time-window","days":["Mon","Tue","Wed","Thu","Fri"],"start":"09:00","end":"17:00","tz":"UTC"}`

	// Fri 2026-05-15 12:00 UTC — inside.
	in := policy.EvaluationInput{Timestamp: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)}
	if res := mustEval(t, e, code, in); !res.Allowed {
		t.Errorf("midday Friday should be allowed")
	}
	// Sat 2026-05-16 12:00 UTC — wrong day.
	in = policy.EvaluationInput{Timestamp: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)}
	if res := mustEval(t, e, code, in); res.Allowed {
		t.Errorf("Saturday should be denied")
	}
	// Fri 2026-05-15 20:00 UTC — after window.
	in = policy.EvaluationInput{Timestamp: time.Date(2026, 5, 15, 20, 0, 0, 0, time.UTC)}
	if res := mustEval(t, e, code, in); res.Allowed {
		t.Errorf("20:00 should be outside the 09-17 window")
	}
}

func TestBuiltin_TimeWindow_TZApplied(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()
	// 09-17 America/New_York. 13:00 UTC = 09:00 EDT (May, DST) → inside.
	code := `{"rule":"time-window","start":"09:00","end":"17:00","tz":"America/New_York"}`
	in := policy.EvaluationInput{Timestamp: time.Date(2026, 5, 15, 13, 0, 0, 0, time.UTC)}
	if res := mustEval(t, e, code, in); !res.Allowed {
		t.Errorf("13:00 UTC = 09:00 EDT should be inside the window")
	}
	// 12:00 UTC = 08:00 EDT → before window.
	in = policy.EvaluationInput{Timestamp: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)}
	if res := mustEval(t, e, code, in); res.Allowed {
		t.Errorf("12:00 UTC = 08:00 EDT should be before the window")
	}
}

func TestBuiltin_ConfigErrors(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()
	cases := []string{
		`not json at all`,
		`{"rule":"does-not-exist"}`,
		`{"rule":"require-labels"}`,                       // empty keys
		`{"rule":"allowed-actions","allowed":[]}`,         // empty allowed
		`{"rule":"resource-quota","max":10}`,              // missing field
		`{"rule":"pattern-allow","field":"x"}`,            // no patterns
		`{"rule":"pattern-deny","field":"x","patterns":["["]}`, // bad glob
		`{"rule":"time-window","tz":"Not/AZone"}`,         // bad tz
		`{"rule":"time-window","start":"9am"}`,            // bad start fmt
		`{"rule":"time-window","days":["Funday"]}`,        // bad day
	}
	for _, code := range cases {
		_, err := e.Evaluate(context.Background(), biPolicy("p", code), policy.EvaluationInput{})
		if err == nil {
			t.Errorf("config %q: expected error", code)
			continue
		}
		if !errors.Is(err, policy.ErrInvalidPolicy) {
			t.Errorf("config %q: err not ErrInvalidPolicy family: %v", code, err)
		}
	}
}

func TestBuiltin_SourceContextVsResource(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()
	// deny-privileged reading from context instead of resource.
	code := `{"rule":"deny-privileged","field":"priv","source":"context"}`
	res := mustEval(t, e, code, policy.EvaluationInput{
		Resource: map[string]any{"priv": true},          // ignored
		Context:  map[string]any{"priv": false},          // checked
	})
	if !res.Allowed {
		t.Errorf("source=context should read context.priv (false) → allow")
	}
}

func TestBuiltin_CacheReuseAndRecompile(t *testing.T) {
	t.Parallel()
	e := policy.NewBuiltinEvaluator()
	p := biPolicy("c", `{"rule":"allowed-actions","allowed":["read"]}`)
	for i := 0; i < 5; i++ {
		res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{Action: "read"})
		if err != nil || !res.Allowed {
			t.Fatalf("iter %d: res=%+v err=%v", i, res, err)
		}
	}
	// Same ID, different Code → different cache key → recompiled.
	p.Code = `{"rule":"allowed-actions","allowed":["write"]}`
	res, err := e.Evaluate(context.Background(), p, policy.EvaluationInput{Action: "read"})
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if res.Allowed {
		t.Errorf("changed code not recompiled (read still allowed under write-only)")
	}
}

func TestBuiltin_SatisfiesInterface(t *testing.T) {
	t.Parallel()
	var _ policy.Evaluator = policy.NewBuiltinEvaluator()
}
