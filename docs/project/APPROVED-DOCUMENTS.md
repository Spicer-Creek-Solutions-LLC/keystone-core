# Approved document snapshots

This is an approval ledger, not a claim that Git history can infer approval.
`tools/doclint -approved-documents` compares each listed document with the
content digest and snapshot commit explicitly recorded here and prints the
complete divergence for review.

The check is part of `make check`. If a listed document changes, its ledger
entry must change in the same pull request. The SHA-256 digest makes that
decision checkable before the eventual merge commit exists; the snapshot commit
identifies the last sanctioned version and does not need to be the merge commit
of the current pull request.

Each entry needs the document path, the full last-sanctioned snapshot commit, the
exact current-content SHA-256 digest, and a short reason. Updating an entry is
itself a review decision. The check reads the working tree so a local change is
visible before it is committed; CI evaluates the checked-out pull-request tree.
