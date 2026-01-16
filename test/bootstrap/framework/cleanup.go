package framework

import "context"

// Cleanup runs teardown actions after a test run.
func Cleanup(ctx context.Context, funcs ...func(context.Context) error) error {
	for _, fn := range funcs {
		if fn == nil {
			continue
		}
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}
