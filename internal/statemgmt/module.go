package statemgmt

import "context"

// Module is the minimal contract implemented by every state type in
// the stdlib (file, package, service, ...). The Check / Apply / Test
// triplet is the non-negotiable idempotency pattern from
// PROJECT-DETAILS §4.8: Check diffs, Apply converges, Test verifies.
//
// Module implementations MUST be safe to call concurrently across
// different Declarations. Sharing the same Declaration across
// goroutines is the runner's responsibility and is not supported.
type Module interface {
	// Name returns the module's registry name, e.g. "file". It must
	// be stable and match the name passed to RegisterModule.
	Name() string

	// ValidStates returns the set of values a Declaration.State may
	// take for this module, e.g. {"present", "absent"} for the file
	// module. The validator (Task 4) rejects declarations whose
	// State is not in this set.
	ValidStates() []string

	// Check inspects the live system and reports whether the
	// Declaration is already satisfied. It must not mutate state.
	Check(ctx context.Context, decl *Declaration) (*ModuleCheckResult, error)

	// Apply converges the live system to the Declaration. It must
	// be idempotent: applying the same Declaration twice in a row
	// must report Changed=false on the second call.
	Apply(ctx context.Context, decl *Declaration) (*StateResult, error)

	// Test verifies the live system matches the Declaration after
	// Apply. It is the post-condition check that closes the
	// idempotency loop.
	Test(ctx context.Context, decl *Declaration) (bool, error)
}

// Factory constructs a Module. The Registry stores factories rather
// than singletons so each Get returns a fresh instance — modules may
// hold per-run state (caches, dispatched providers) without leaking
// across runs.
type Factory func() Module
