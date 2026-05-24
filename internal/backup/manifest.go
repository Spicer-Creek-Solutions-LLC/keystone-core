// SPDX-License-Identifier: Apache-2.0

package backup

import "time"

// ManifestFormatV1 is the only manifest format known to v1.0. A v2
// would introduce a new constant and a version-discriminated decoder
// in the future Restore path.
const ManifestFormatV1 = 1

// ManifestFilename is the fixed name of the manifest entry inside the
// backup tar.
const ManifestFilename = "manifest.json"

// Manifest is the index of a backup artifact: timestamp, cluster
// identifier, and a per-component entry that lets a restore reader
// locate each blob inside the tar and verify its integrity by
// re-hashing the entry bytes against [ComponentEntry.SHA256Hex].
type Manifest struct {
	FormatVersion int               `json:"format_version"`
	TakenAt       time.Time         `json:"taken_at"`
	ClusterName   string            `json:"cluster_name,omitempty"`
	Components    []ComponentEntry  `json:"components"`
}

// ComponentEntry is one row of [Manifest.Components]. Name is the
// stable component identifier (e.g. "storage", "config") — Restore
// dispatches on it. Path is the tar entry path. Size + SHA256Hex are
// computed by [BackupManager.CreateBackup] as the component streams
// through the SHA-256 tee.
type ComponentEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	SHA256Hex string `json:"sha256"`
}
