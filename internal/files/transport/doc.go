// Package transport wires the kscore file service onto NATS. It is
// the chunked-streaming layer between a [backend.Store] (Task 10)
// and consumers — REST handlers (Task 14), the kscore-files CLI
// (Task 15), and the agent proxy cache (Task 12).
//
// Two structs make up the surface:
//
//	Service   server-side; subscribes to files.request.* and
//	          dispatches put / get / list / delete / stat against
//	          a backend.Store. One Service per file-service node.
//
//	Client    client-side; sends requests + reassembles chunked
//	          downloads + publishes chunked uploads. Methods
//	          mirror the backend.Store interface so callers can
//	          treat the bus as a remote backend.
//
// Wire protocol (NATS Core pub-sub):
//
//	GET
//	  1. Client subscribes to files.response.<reqID> +
//	     files.chunk.<transferID>.
//	  2. Client publishes FileRequest{op=get, path, from_chunk?} on
//	     files.request.get with headers Kscore-Request-Id +
//	     Kscore-Transfer-Id.
//	  3. Service publishes FileResponse{Metadata, Total} on the
//	     response subject. On error: FileResponse{Error}.
//	  4. Service streams chunks FromChunk..Total-1 on the chunk
//	     subject. Each chunk carries SHA-256 of its Data.
//	  5. Client reassembles + verifies per-chunk hash + verifies
//	     assembled hash against Metadata.Hash.
//
//	PUT
//	  1. Client subscribes to files.response.<reqID>.
//	  2. Client publishes FileRequest{op=put, path, metadata} on
//	     files.request.put with the two headers.
//	  3. Service subscribes to files.chunk.<transferID>, replies on
//	     the response subject with FileResponse{Total, status=ready}.
//	  4. Client publishes chunks 0..Total-1 on the chunk subject.
//	  5. Service accumulates, writes to backend, replies on the
//	     response subject with FileResponse{Metadata: backend-
//	     assigned} (or Error).
//
//	LIST / DELETE / STAT
//	  Single-shot publish on files.request.<op> + reply on
//	  files.response.<reqID>. No chunks.
//
// Resume: only the get direction supports resume in v1.0. Clients
// that lose chunks K..Total-1 mid-transfer reissue Get with
// FromChunk=K; the service re-reads from the backend at that
// offset. Put-side resume is deferred to v1.x (operator-named
// requirement is exfiltration / dr, not packet-loss tolerance on
// big-file upload). See docs/project/ROADMAP.md.
package transport
