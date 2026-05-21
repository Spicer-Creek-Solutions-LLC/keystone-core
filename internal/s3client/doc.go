// Package s3client constructs minio-go S3 clients from a shared
// [Config]. It is the single home of S3 connection state across
// kscore packages so a kscore-backup destination, a kscore-files
// backend, and any future S3-using package all parse the same
// operator-supplied fields and produce structurally identical
// clients.
//
// The "S3" name covers every S3-API-compatible service:
// AWS S3, MinIO, Backblaze B2, Cloudflare R2, Wasabi, DigitalOcean
// Spaces, etc. The service is selected by [Config.Endpoint]:
// empty selects AWS, any other host selects the named service.
package s3client
