// SPDX-License-Identifier: Apache-2.0

// Package backend is the storage layer behind the kscore file
// service. It owns where bytes live (filesystem, S3-compatible
// object store, in-memory test double) without knowing anything
// about the NATS chunked transport on top of it.
//
// Three impls ship in this package:
//
//	MemoryStore       in-memory map, for service-layer unit tests.
//	FilesystemStore   <root>/data + <root>/meta on local disk.
//	S3Store           any S3-API-compatible bucket.
//
// All three satisfy [Store] and are exercised by the same
// conformance test set (see conformance_test.go). That conformance
// suite is the authoritative description of the contract.
//
// Versioning model is "single latest, monotonic counter":
//
//	Put always assigns Version = previous + 1 (or 1 if absent).
//	Get / Stat / List return the latest version only.
//	Delete removes the file.
//
// Per-version retention and time-travel reads are v1.x concerns,
// not v1.0 (see PROJECT-DETAILS sec 4.20).
//
// The [files.FileMetadata.Path] format is canonical across the
// package: forward slashes, no leading slash, no traversal. The
// backend takes care of mapping that to OS-native paths and to S3
// keys, applying defense-in-depth checks on every operation.
package backend
