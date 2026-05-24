// SPDX-License-Identifier: Apache-2.0

// Package files contains the file-distribution wire-format contract
// shared by the kscore-server file service, the kscore-agent proxy
// cache, and the kscore-files CLI. Epic 18 task 9 ships only the
// types + NATS subject conventions; subsequent tasks layer on:
//
//	task 10  BackendStore interface + filesystem + S3 backends
//	task 11  chunked NATS streaming with per-chunk SHA-256 + resume
//	task 12  LRU+TTL proxy cache on agents
//	task 13  namespace ACLs wired to RBAC
//	task 14  REST handlers in pkg/api/files
//	task 15  kscore-files CLI
//
// The wire-format invariants:
//
//	FileMetadata.Path       slash-delimited; no leading slash, no
//	                        "..", no dots inside tokens (NATS
//	                        wildcards), no whitespace.
//	FileMetadata.Hash       hex SHA-256 of the assembled body.
//	FileChunk.Index < Total assembled in order; reassembly verifies
//	                        the per-chunk hash and the total hash.
//	FileChunk.Data          up to [ChunkSize] (1 MiB default) bytes.
package files
