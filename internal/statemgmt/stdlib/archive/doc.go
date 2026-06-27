// SPDX-License-Identifier: Apache-2.0

package archive

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the archive module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Files & VCS",
		Summary: "Extracts an archive (tar, tar.gz, tar.bz2, or zip) into a target " +
			"directory. Idempotent: a sentinel recording the archive's size and " +
			"mtime (or a `creates` path) lets re-applying an unchanged declaration " +
			"report no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The archive's contents are extracted under `target`; re-extracted when the archive's size or mtime changes (or, with `creates`, only when the `creates` path is missing)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "target", Type: "string", Required: true, Desc: "Directory to extract into; created (mode 0755) if absent. The declaration name is the archive file path."},
			{Name: "format", Type: "string", Default: "auto", Desc: "Archive format: `auto`, `tar`, `tar.gz` (`tgz`), `tar.bz2` (`tbz2`/`tbz`), or `zip`. `auto` infers from the filename extension."},
			{Name: "creates", Type: "string", Desc: "Idempotency short-circuit: if this path exists (absolute, or relative to `target`) the archive is considered already extracted and is never read."},
			{Name: "strip_components", Type: "int", Default: "0", Desc: "Strip this many leading path components from each archived entry (like `tar --strip-components`)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Extract a tarball into a directory",
				YAML: `archive:
  /var/cache/myapp/release.tar.gz:
    state: present
    target: /opt/myapp`,
			},
			{
				Title: "Fetch then extract, with a creates guard",
				Desc:  "The `require` orders the extraction after the file fetch; `creates` skips re-extraction once the marker exists.",
				YAML: `archive:
  /var/cache/myapp/release.tar.gz:
    state: present
    target: /opt/myapp
    format: tar.gz
    strip_components: 1
    creates: /opt/myapp/bin/myapp
    require:
      - file: /var/cache/myapp/release.tar.gz`,
			},
			{
				Title: "Unzip a bundle",
				YAML: `archive:
  /srv/dist/site.zip:
    state: present
    target: /srv/www
    format: zip`,
			},
		},
		Notes: []string{
			"OS-agnostic: extraction uses the Go stdlib (no external `tar`/`unzip` binary).",
			"Stable, extract-only: the sole state is `present`. Removing an extracted tree is out of scope — use the `file` module with `state: absent` on the target.",
			"Supported formats are tar, tar.gz (tgz), tar.bz2 (tbz2/tbz), and zip. `.tar.xz`, `.tar.zst`, and `.7z` are not supported.",
			"Every entry path is validated to stay within `target` (zip-slip / `..` defense); symlink, hardlink, and device entries are skipped, not created.",
			"Re-extraction overwrites archived files in place; files in `target` that are not in the archive are left untouched (no `clean` mode). Touching the archive without changing its contents can trigger a needless re-extract, since identity is size+mtime, not a content hash.",
			"chown/owner/group and mtime preservation of the extracted tree, and extraction size/entry-count (zip-bomb) limits, are out of scope for v1.0.",
		},
	}
}
