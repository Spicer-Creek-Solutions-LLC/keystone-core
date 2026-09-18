# Approved document snapshots

This is an approval ledger, not a claim that Git history can infer approval.
`tools/doclint -approved-documents` compares each listed document with the
content digest and snapshot commit explicitly recorded here and prints the
complete divergence for review.

The check is part of `make check`. If a listed document changes, its ledger
entry must change in the same pull request. The SHA-256 digest is the gate and
makes that decision checkable before the eventual merge commit exists. The
snapshot commit identifies the last sanctioned version for provenance; it may
become unreachable after a rebase. An unavailable snapshot is reported with a
warning, while the content digest remains authoritative.

Each entry needs the document path, the full last-sanctioned snapshot commit, the
exact current-content SHA-256 digest, and a short reason. Updating an entry is
itself a review decision. The check reads the working tree so a local change is
visible before it is committed; CI evaluates the checked-out pull-request tree.
The snapshot commit should contain the sanctioned content when one is available,
but it is not a second approval source.
