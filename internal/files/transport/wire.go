package transport

import (
	"go.keystone-core.io/keystone-core/internal/files"
)

// Header names carried on NATS messages. NATS headers are
// case-insensitive on the wire; we standardise on Title-Case here.
const (
	HeaderRequestID  = "Kscore-Request-Id"
	HeaderTransferID = "Kscore-Transfer-Id"
)

// ResponseStatus enumerates the lifecycle stages a FileResponse
// might announce. Operations that complete in a single reply use
// StatusDone; the put flow uses StatusReady to signal "service is
// subscribed to the chunk subject, send chunks now".
type ResponseStatus string

const (
	StatusReady ResponseStatus = "ready"
	StatusDone  ResponseStatus = "done"
)

// FileResponse is the server-side reply published on
// files.response.<reqID>. Exactly one of Error or the operation-
// specific payload field is set per message.
type FileResponse struct {
	// Status reports which lifecycle stage of the transfer this
	// message represents. For list / delete / stat / put-final
	// the status is StatusDone. For the put kickoff it is
	// StatusReady.
	Status ResponseStatus `json:"status"`

	// Error is non-empty on failure. The transport surfaces it as
	// a Go error in the client's caller.
	Error string `json:"error,omitempty"`

	// Metadata is set on get / stat / put-final responses.
	Metadata *files.FileMetadata `json:"metadata,omitempty"`

	// Total is the number of chunks the chunked transfer will
	// carry. Set on the get-ready and put-ready responses.
	Total int `json:"total,omitempty"`

	// ChunkSize echoes the chunk-size budget so clients building
	// the chunk list know the boundary. v1.0 has it locked at
	// [files.ChunkSize]; sending it in the response lets v1.x
	// evolve without a wire break.
	ChunkSize int `json:"chunk_size,omitempty"`

	// List carries the matching FileMetadata entries for list
	// responses.
	List []files.FileMetadata `json:"list,omitempty"`
}
