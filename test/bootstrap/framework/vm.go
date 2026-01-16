package framework

import "context"

// VMEnv manages VM-based bootstrap test environments.
type VMEnv struct {
	Provider string
}

// NewVMEnv creates a VM environment placeholder.
func NewVMEnv(provider string) *VMEnv {
	return &VMEnv{Provider: provider}
}

// Start prepares VM resources for testing.
func (v *VMEnv) Start(ctx context.Context) error {
	return nil
}

// Stop cleans up VM resources.
func (v *VMEnv) Stop(ctx context.Context) error {
	return nil
}
