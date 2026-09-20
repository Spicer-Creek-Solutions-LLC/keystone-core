# Ledger Schema

This is the published on-disk schema for the agent ledger. It is an
independent artifact: AC-10 compares this text with the schema shipped by the
implementation, rather than comparing two renderings from one source.

The ledger is SQLite, owned by the agent account, at the deployment-selected
path. The containing directory is mode `0700`; the database is mode `0600`.

```sql
CREATE TABLE schema_migrations (
  version INTEGER PRIMARY KEY CHECK(version > 0),
  applied_at INTEGER NOT NULL
);

CREATE TABLE receipts (
  job_id TEXT PRIMARY KEY,
  metadata BLOB NOT NULL,
  state TEXT NOT NULL,
  received_at INTEGER NOT NULL
);

CREATE TABLE starts (
  job_id TEXT PRIMARY KEY REFERENCES receipts(job_id),
  started_at INTEGER NOT NULL
);

CREATE TABLE terminal_results (
  job_id TEXT PRIMARY KEY REFERENCES receipts(job_id),
  state TEXT NOT NULL,
  exit_status INTEGER NOT NULL,
  output BLOB NOT NULL,
  truncated INTEGER NOT NULL,
  discarded_bytes INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL
);

CREATE TABLE result_pubacks (
  job_id TEXT PRIMARY KEY REFERENCES terminal_results(job_id),
  pub_ack BLOB NOT NULL,
  acked_at INTEGER NOT NULL
);
```

Receipt and start are separate writes. A result publication acknowledgement is
also a separate write after the terminal result. The schema records the
ordering obligations from ADR-0006 §3; it does not claim exactly-once execution.
