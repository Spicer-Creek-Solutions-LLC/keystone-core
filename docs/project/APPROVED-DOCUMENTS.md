# Approved document snapshots

This is a review ledger, not a claim that Git history can infer approval.
`tools/doclint -approved-documents` compares each listed document with the
commit explicitly recorded here and prints the complete divergence for review.

The ledger is intentionally not part of `make check`. A document change cannot
know its eventual merge commit before it merges, and a pre-merge gate would
either reject every legitimate document change or silently approve an edit by
moving its own baseline. After a document PR merges, its reviewer records the
approved merge snapshot here. A later unapproved edit then becomes visible when
the review command is run.

Each entry needs the document path, the full commit that approved its current
contents, and a short reason. Updating an entry is itself a review decision.
