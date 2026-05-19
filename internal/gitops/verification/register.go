package verification

import "net/http"

// Deps carries the seams the default verifiers need: the command
// runner (CommandVerifier owns no os/exec) and optional HTTP-client /
// gRPC-health overrides. Zero values are valid — HTTPVerifier falls
// back to a default client and GRPCVerifier to the real health check;
// only CommandRunner has no safe default (a nil runner fails the
// command step at Verify time).
type Deps struct {
	CommandRunner CommandRunner
	HTTPClient    *http.Client
	HealthCheck   HealthChecker
}

// RegisterAll binds the three v1.0 verifiers (http, grpc, command)
// onto reg using deps. Single wiring point; mirrors the webhook /
// runbook RegisterAll pattern so the set is enumerated in one place.
func RegisterAll(reg *Registry, deps Deps) error {
	verifiers := []Verifier{
		HTTPVerifier{Client: deps.HTTPClient},
		GRPCVerifier{Check: deps.HealthCheck},
		CommandVerifier{Runner: deps.CommandRunner},
	}
	for _, v := range verifiers {
		if err := reg.Register(v); err != nil {
			return err
		}
	}
	return nil
}

// NewDefaultRegistry returns a [Registry] with the three v1.0
// verifiers registered using deps. Panics only on a programming error
// (a verifier reporting an empty type), which the built-in set cannot
// hit — kept as a guard for future additions.
func NewDefaultRegistry(deps Deps) *Registry {
	reg := NewRegistry()
	if err := RegisterAll(reg, deps); err != nil {
		panic("verification: built-in verifier registration failed: " + err.Error())
	}
	return reg
}
