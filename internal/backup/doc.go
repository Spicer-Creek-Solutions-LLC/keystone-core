// SPDX-License-Identifier: Apache-2.0

// Package backup orchestrates portable kscore-server backup
// artifacts. Epic 18 task 3 ships the orchestrator and six narrow
// component seams ([StorageBackup], [JetStreamBackup], [EtcdBackup],
// [ConfigCollector], [SecretsBackup], [ClusterMetadata]); real
// adapter implementations land in follow-on tasks tracked under the
// gate-v1.0 ROADMAP entry "Backup component adapters".
//
// The artifact is an uncompressed tar containing per-component blobs
// under `components/` and a single `manifest.json` that records the
// taken-at timestamp, cluster name, and per-component path + size +
// SHA-256 hex. Epic 18 task 4 layers age encryption around the tar;
// task 5 adds destination backends (local/S3); task 6 builds the
// matching restore + verify path.
package backup
