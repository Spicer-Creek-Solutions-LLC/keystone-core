// Package testing provides reusable test utilities and mock implementations
// for Keystone Core unit and integration tests.
//
// The testing package enables test isolation, deterministic behavior, and
// common testing patterns without requiring external dependencies. It is
// used throughout the Keystone Core test suites.
//
// # Subpackages
//
//   - helpers: Utility functions for common test patterns
//   - mocks: Mock implementations for testing component interactions
//
// # Wait Helpers
//
// The helpers subpackage provides utilities for polling-based assertions:
//
//	import "github.com/your-org/keystone-core/pkg/testing/helpers"
//
//	// Wait for condition with context
//	err := helpers.WaitForCondition(ctx, 100*time.Millisecond, func() (bool, error) {
//	    return server.IsReady(), nil
//	})
//
//	// Wait for condition with timeout
//	err := helpers.WaitForTimeout(5*time.Second, 100*time.Millisecond, func() (bool, error) {
//	    return len(results) > 0, nil
//	})
//
// # Mock Implementations
//
// The mocks subpackage provides mock implementations for testing:
//
// NATS Mocks:
//
//	import "github.com/your-org/keystone-core/pkg/testing/mocks"
//
//	// Status provider mock
//	statusMock := &mocks.NATSStatusProvider{
//	    Connected:  true,
//	    URLs:       []string{"nats://localhost:4222"},
//	    JetStream:  true,
//	}
//
//	// Controller mock with error injection
//	ctrlMock := &mocks.NATSController{
//	    RestartErr: errors.New("restart failed"),
//	}
//
// Agent Store Mock:
//
//	agentStore := &mocks.AgentStore{}
//	agentStore.SaveAgent(ctx, agent)
//	agents, err := agentStore.ListAgents(ctx)
//
// The agent store mock maintains an in-memory map and supports configurable
// error injection for failure testing.
//
// # Usage Guidelines
//
// When writing tests:
//
//  1. Use t.TempDir() for filesystem isolation
//  2. Use mocks from this package for external dependencies
//  3. Use WaitForCondition for async assertions instead of time.Sleep
//  4. Configure error fields on mocks for failure path testing
//
// Example test:
//
//	func TestComponent(t *testing.T) {
//	    store := &mocks.AgentStore{}
//	    component := NewComponent(store)
//
//	    // Test success path
//	    result, err := component.DoWork(ctx)
//	    require.NoError(t, err)
//
//	    // Test failure path
//	    store.GetErr = errors.New("not found")
//	    _, err = component.DoWork(ctx)
//	    require.Error(t, err)
//	}
package testing
