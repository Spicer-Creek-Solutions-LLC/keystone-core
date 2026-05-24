// SPDX-License-Identifier: Apache-2.0

// Package profiling runs the opt-in pprof endpoint on a dedicated
// listener separate from the main HTTP server.
//
// The package never imports net/http/pprof for its side effect — that
// import registers handlers on http.DefaultServeMux, which pollutes the
// global state every test in this process shares. Instead, Server
// constructs its own *http.ServeMux and registers the canonical pprof
// handlers (Index / Cmdline / Profile / Symbol / Trace) explicitly.
// The Index handler internally exposes heap, goroutine, block, mutex,
// threadcreate, and allocs profiles via runtime/pprof's named registry.
//
// Defaults are conservative:
//
//   - Enabled=false. pprof leaks heap pointers and can degrade
//     application performance under CPU-profile load; opt-in is the
//     only safe stance.
//   - Host=127.0.0.1. Operators who want LAN reachability set
//     0.0.0.0 explicitly.
//   - MutexProfileFraction=0, BlockProfileRate=0. The runtime defaults.
//     Non-zero values have measurable overhead; enable only when
//     debugging contention.
//
// No auth — pprof's handlers stream large bodies, and middleware that
// buffers responses interacts badly with the streaming CPU/trace
// profile handlers. Operators rely on network isolation (the localhost
// default) or an SSH tunnel for remote access.
package profiling
