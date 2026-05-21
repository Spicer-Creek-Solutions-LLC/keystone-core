package files

// Subjects is the narrow interface internal/files consumers depend on
// to build cluster-scoped NATS subjects without importing
// internal/nats. internal/nats.SubjectBuilder satisfies it
// structurally; tests can stub it without pulling in the broader
// NATS surface.
//
// The five methods cover the four subject forms documented in
// internal/nats/subject.go's file-distribution block:
//
//	FilesRequest(op)        kscore.<cluster>.files.request.<op>
//	FilesRequestPattern()   kscore.<cluster>.files.request.*
//	FilesResponse(reqID)    kscore.<cluster>.files.response.<reqID>
//	FilesChunk(transferID)  kscore.<cluster>.files.chunk.<transferID>
//	FilesMetadata()         kscore.<cluster>.files.metadata
type Subjects interface {
	FilesRequest(op string) string
	FilesRequestPattern() string
	FilesResponse(requestID string) string
	FilesChunk(transferID string) string
	FilesMetadata() string
}
