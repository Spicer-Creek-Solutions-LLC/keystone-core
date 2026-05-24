// SPDX-License-Identifier: Apache-2.0

package state

// Compile-time interface compliance assertions. If a method is added to
// Store and a backend forgets to implement it, this file fails to compile.
var (
	_ Store = (*SQLiteStore)(nil)
	_ Store = (*PostgreSQLStore)(nil)

	_ AgentStore    = (*SQLiteStore)(nil)
	_ CommandStore  = (*SQLiteStore)(nil)
	_ BatchJobStore = (*SQLiteStore)(nil)
	_ HealthStore   = (*SQLiteStore)(nil)

	_ AgentStore    = (*PostgreSQLStore)(nil)
	_ CommandStore  = (*PostgreSQLStore)(nil)
	_ BatchJobStore = (*PostgreSQLStore)(nil)
	_ HealthStore   = (*PostgreSQLStore)(nil)
)
