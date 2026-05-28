# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Pending entries for the next release live as YAML fragments under
[`.changes/unreleased/`](.changes/unreleased/). They roll up into a
versioned section here via `make changelog-batch VERSION=v0.x.y` at
release time. Preview the accumulated set without writing the file
via `make changelog-preview`.

Per-PR workflow: instead of editing this section directly, run `make
changelog-new` (or `changie new`) to create a fragment file. See
[CONTRIBUTING.md § Changelog entries](CONTRIBUTING.md#changelog-entries).

## [v1.0.0] — Planned

Pending all 19 epics complete + the v1.0 gate checklist in
[`docs/project/VERSIONING.md`](docs/project/VERSIONING.md). The full v1.0.0
entry will land with the v1.0 cut; the in-progress feature inventory tracks
under [`FEATURES.md`](FEATURES.md). Until then, v0.x is the active release
line per the v0.1 → v0.5 → v1.0 ladder.
