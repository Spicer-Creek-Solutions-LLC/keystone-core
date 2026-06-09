// SPDX-License-Identifier: Apache-2.0

//go:build integration && race

package main

// raceEnabled is true when the test binary is built with the race
// detector (`go test -race`). Integration tests use it to skip latency
// SLO assertions that the race detector's scheduling overhead makes
// unrepresentative.
const raceEnabled = true
