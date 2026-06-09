// SPDX-License-Identifier: Apache-2.0

//go:build integration && !race

package main

// raceEnabled is false in a non-race integration build, where latency
// SLO assertions are meaningful. See race_on_test.go.
const raceEnabled = false
