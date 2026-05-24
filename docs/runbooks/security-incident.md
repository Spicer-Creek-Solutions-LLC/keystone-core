# Runbook: Security Incident Response

## Overview

Response procedures for security incidents affecting a Keystone Core
v0.1 deployment. v0.1 is single-node — "incident response" operates
on one server host plus N agent hosts, using the primitives shipped
in v0.1: `systemctl`, the audit log, the events stream, the identity
CLI, the REST API key endpoints, `kscore-cluster-backup`, and
standard Linux forensics tooling.

> **v0.x scope note**: v0.1 does not ship agent-quarantine commands,
> a cluster-membership API, an audit-log "search/analyze/timeline"
> surface, or a packaged incident-bundle generator. Where this
> runbook reaches for those capabilities it falls back to the
> primitives that exist — `kscorectl audit log` filters,
> `kscorectl events list/replay`, `kscorectl identity ca
> rotate-signing`, REST `DELETE /api/v1/apikeys/{id}` via `curl`, and
> per-host `systemctl stop`. The orchestrated surfaces are tracked
> in [`docs/project/ROADMAP.md`](../project/ROADMAP.md).

## Trigger conditions

Execute this runbook when one of the following is observed:

- Unauthorized API access — calls succeeding with an API key the
  operator did not issue, or unexpected `role=admin` activity
- Credential compromise suspected (API key, agent bootstrap PSK,
  SPIFFE/JWT identity token, or signing-CA material)
- Anomalous agent activity — agents executing commands the operator
  did not dispatch, or agents reconnecting under an identity that
  no longer matches their known fingerprint
- Policy violations indicating attack — a sustained run of
  policy-deny events in the same window from the same principal
- External security disclosure (researcher report, dependency CVE
  with confirmed exposure, downstream sighting)
- File-integrity drift on `/etc/kscore/`, `/var/lib/kscore/`, or
  `/usr/bin/kscore-*` that the operator did not perform

## Severity classification

Vendor-neutral severity matrix; teams adapt the response-time column
to their SLAs.

| Sev   | Description                                  | Response time | Examples |
|-------|----------------------------------------------|--------------|----------|
| Sev-0 | Active breach, exfiltration in progress      | Immediate    | Unauthorized admin API key in active use; agent fleet executing attacker-supplied commands |
| Sev-1 | Confirmed compromise, no active exploitation | < 1 hour     | Leaked API key disclosed by researcher; signing-CA private key exposure |
| Sev-2 | Suspicious activity, unconfirmed             | < 4 hours    | Auth-failure spike from a single IP; unexplained policy-deny burst |
| Sev-3 | Security finding, no active risk             | < 24 hours   | Dependency CVE for an unused code path; vulnerability scan output |

When in doubt, escalate one level. Document the decision in the
incident ticket so the post-mortem can review the call.

---

## Phase 1 — Immediate response (stop the bleed)

### 1.1 Open the incident record

```bash
INCIDENT=incident-$(date -u +%Y%m%dT%H%M%S)
EVIDENCE=/secure/${INCIDENT}/evidence
sudo mkdir -p "$EVIDENCE"
sudo chmod 0700 "$(dirname "$EVIDENCE")"

cat | sudo tee "$EVIDENCE/00-observations.txt" >/dev/null <<EOF
Incident: ${INCIDENT}
Started:  $(date -u +"%Y-%m-%dT%H:%M:%SZ")
Reporter: <name>
Detection: <how was it noticed>
Initial scope: <hosts, principals, time window>
Initial severity: Sev-<n>
EOF
```

Keep the evidence directory off the affected host once you have an
isolated forensics target — `scp`/`rsync` it out before doing
anything destructive on the source.

### 1.2 Isolate

If the server itself is suspected compromised:

```bash
# Stop the control plane. systemd drains in-flight gRPC streams
# within TimeoutStopSec=30s.
sudo systemctl stop kscore-server

# Block all but the operator's SSH source so the host is
# inspectable but unreachable to attacker traffic. Adjust to
# match your firewall (nftables / iptables / firewalld).
sudo iptables -I INPUT 1 -p tcp --dport 22 -s <operator-ip> -j ACCEPT
sudo iptables -A INPUT -j DROP
```

If specific agent hosts are suspected:

```bash
# On each suspect agent host:
sudo systemctl stop kscore-agent

# Optionally block its outbound NATS to ensure it can't reconnect
# until investigation is complete.
sudo iptables -A OUTPUT -p tcp --dport 4222 -j DROP
```

v0.1 has no `kscorectl agents quarantine`; the host-level stop is
the v0.1 equivalent. The orchestrated quarantine surface is tracked
in [`docs/project/ROADMAP.md`](../project/ROADMAP.md).

### 1.3 Capture volatile evidence

Run this on the affected host(s) BEFORE further inspection mutates
state. Process lists, open sockets, and memory-resident state are
lost on reboot.

```bash
# Process tree.
sudo ps auxf > "$EVIDENCE/$(hostname)-processes.txt"

# Open sockets with owning PIDs.
sudo ss -tuanp > "$EVIDENCE/$(hostname)-sockets.txt"

# Open files (large; trim post-hoc).
sudo lsof -n > "$EVIDENCE/$(hostname)-lsof.txt"

# Logged-in users + recent logins.
who   > "$EVIDENCE/$(hostname)-who.txt"
last -20 > "$EVIDENCE/$(hostname)-last.txt"

# Recent journal lines for the kscore units (no truncation).
sudo journalctl -u kscore-server --since "24 hours ago" --no-pager \
    > "$EVIDENCE/$(hostname)-server.log" 2>/dev/null || true
sudo journalctl -u kscore-agent  --since "24 hours ago" --no-pager \
    > "$EVIDENCE/$(hostname)-agent.log"  2>/dev/null || true

# Full journal (auth, sudo, ssh attempts).
sudo journalctl --since "24 hours ago" --no-pager \
    > "$EVIDENCE/$(hostname)-journal.log"

# auditd events (if installed).
sudo ausearch -ts recent > "$EVIDENCE/$(hostname)-ausearch.txt" 2>/dev/null || true
```

### 1.4 Snapshot kscore configuration and state

```bash
# Config: server.yaml + agent.yaml as on-disk (REDACT BEFORE
# SHARING -- the HMAC secret + master key live here).
sudo cp /etc/kscore/server.yaml "$EVIDENCE/server.yaml.at-incident" 2>/dev/null || true
sudo cp /etc/kscore/agent.yaml  "$EVIDENCE/agent.yaml.at-incident"  2>/dev/null || true

# State DB (SQLite default). Copy with the server stopped so the
# WAL doesn't move under you.
sudo cp /var/lib/kscore/keystone.db    "$EVIDENCE/keystone.db.at-incident"    2>/dev/null || true
sudo cp /var/lib/kscore/keystone.db-wal "$EVIDENCE/keystone.db-wal.at-incident" 2>/dev/null || true
sudo cp /var/lib/kscore/keystone.db-shm "$EVIDENCE/keystone.db-shm.at-incident" 2>/dev/null || true

# JetStream embedded store (events + audit streams).
sudo tar czf "$EVIDENCE/jetstream.tar.gz" -C /var/lib/kscore jetstream 2>/dev/null || true

# Installed binaries (hash + mtime -- was kscore-server replaced?).
for bin in /usr/bin/kscore-server /usr/bin/kscore-agent /usr/bin/kscorectl \
           /usr/bin/kscore-audit /usr/bin/kscore-events /usr/bin/kscore-identity \
           /usr/bin/kscore-policy /usr/bin/kscore-secrets /usr/bin/kscore-cluster-backup; do
    [ -f "$bin" ] && sha256sum "$bin" >> "$EVIDENCE/binary-hashes.txt"
    [ -f "$bin" ] && ls -la "$bin"   >> "$EVIDENCE/binary-mtimes.txt"
done

# Installed package versions.
sudo dpkg -l kscore-server kscore-cli kscore-agent > "$EVIDENCE/installed.txt" 2>/dev/null || \
    sudo rpm -q kscore-server kscore-cli kscore-agent > "$EVIDENCE/installed.txt"
```

### 1.5 Preserve the audit chain

The audit log is the primary post-incident source of truth. v0.1
writes audit entries to the configured store (SQLite by default,
Postgres optional) as append-only rows; the public API exposes only
read paths (`kscorectl audit log` / `audit export` / `audit report`
/ `audit stats`) — no mutate/delete API exists.

That means: if the DB row count for the audit table decreases
between two snapshots, somebody has direct DB access — a Sev-0
escalation. Snapshot the row count now so you have a baseline.

```bash
# SQLite row count snapshot.
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db \
    "SELECT COUNT(*) FROM audit_events" > "$EVIDENCE/audit-rowcount-pre.txt"

# Export the last 7 days of audit entries to a sealed file. The
# export subcommand was added in Epic 12 task 15.
kscorectl audit export \
    --format jsonl \
    --since 7d \
    --output-file "$EVIDENCE/audit-7d.jsonl"

sha256sum "$EVIDENCE/audit-7d.jsonl" > "$EVIDENCE/audit-7d.jsonl.sha256"
```

### 1.6 Notify

Per your team's escalation matrix. The maintainer-side contact for
**external** disclosures is in [`SECURITY.md`](../../SECURITY.md);
that is NOT a 24/7 hotline — it is the disclosure pipeline for bugs
in keystone-core itself, not your operational incident channel.

For operational notification (PagerDuty / Opsgenie / Slack /
phone tree), use whatever your organization already runs — v0.1
does not ship a notifier integration.

---

## Phase 2 — Investigation

### 2.1 Query the audit log

`kscorectl audit log` supports `--since`, `--until`, `--user`,
`--action`, `--resource-type`, `--policy-id`, `--min-severity`,
`--limit`, `--cursor`. Combine with `jq` for ad-hoc analysis.

```bash
# All actions by a specific principal in the suspect window.
kscorectl audit log \
    --user "<principal>" \
    --since "2026-05-24T00:00:00Z" \
    --until "2026-05-24T06:00:00Z" \
    -o json | tee "$EVIDENCE/audit-by-user.json"

# All actions matching an action pattern (e.g. anything that
# touched secrets).
kscorectl audit log \
    --action "secret.read" \
    --since 24h \
    -o json | tee "$EVIDENCE/audit-secret-reads.json"

# High-severity events only.
kscorectl audit log \
    --min-severity high \
    --since 7d \
    -o json | tee "$EVIDENCE/audit-high-severity.json"

# Tail-style live monitor while investigation runs.
watch -n 10 'kscorectl audit log --limit 20'
```

v0.1 does NOT ship `audit search`, `audit analyze`, or
`audit timeline`. The free-form query surface is post-v1.0 work
in [`docs/project/ROADMAP.md`](../project/ROADMAP.md); for v0.1,
export + `jq` is the timeline-construction path.

```bash
# Construct an ordered timeline from the export.
cat "$EVIDENCE/audit-7d.jsonl" | \
    jq -s 'sort_by(.timestamp) | .[] |
           [.timestamp, .user, .action, .resource_type, .resource_id, .outcome] | @tsv' \
    > "$EVIDENCE/audit-timeline.tsv"
```

### 2.2 Inspect the event stream

The events stream emits real-time signals — agent connects,
disconnects, command dispatches, state-apply outcomes, NATS health,
policy decisions. Suspicious activity that bypasses higher-level
auth is often visible here.

```bash
# Live tail.
kscorectl events watch --since 5m

# Connection / disconnection patterns.
kscorectl events list --type agent_connected    --since 24h
kscorectl events list --type agent_disconnected --since 24h

# Command-exec events tied to a suspect agent.
kscorectl events list --category job --since 24h -o json \
    | jq 'select(.source == "<agent-id>")'

# Severity-filtered replay over a wider window.
kscorectl events replay \
    --since 7d \
    --min-severity error \
    -o json > "$EVIDENCE/events-7d-errors.json"
```

### 2.3 Inspect identity state

```bash
# CA inventory + current signing-CA fingerprint.
kscorectl identity ca info -o json > "$EVIDENCE/ca-info.json"

# IdentityService runtime status (IdentityService gRPC IS
# registered in v0.1, unlike ClusterGRPCServer).
kscorectl identity status -o json > "$EVIDENCE/identity-status.json"

# Outstanding bootstrap tokens. Unused tokens that should have
# been one-shot consumed are red flags.
kscorectl identity token list -o json > "$EVIDENCE/tokens-all.json"
kscorectl identity token list --unused    -o json > "$EVIDENCE/tokens-unused.json"
kscorectl identity token list --unexpired -o json > "$EVIDENCE/tokens-unexpired.json"
```

### 2.4 Inspect API keys

```bash
# v0.1 exposes API key CRUD as REST under /api/v1/apikeys (mounted
# by pkg/api/server/handlers.go); there is no kscorectl apikey
# subcommand surface yet. Use the REST endpoint directly with the
# admin Bearer key. Substitute :8080 if you changed the HTTP port.

API_KEY=<your-admin-bearer-token>
curl -sS -H "Authorization: Bearer $API_KEY" \
    http://127.0.0.1:8080/api/v1/apikeys | jq > "$EVIDENCE/apikeys.json"

# Look for: keys you didn't create, keys created in the incident
# window, keys with role=admin you don't recognise.
jq '.apikeys[] | select(.created_at > "2026-05-24T00:00:00Z")' \
    "$EVIDENCE/apikeys.json"
```

### 2.5 Standard Linux forensics

```bash
# File-integrity check: were any kscore binaries replaced?
# Compare against a known-good hash database -- for v0.1 the
# release artifacts ship with sha256 sidecars in the same
# location as the .deb / .rpm.
sha256sum -c /tmp/known-good-hashes.txt

# Find files modified in the incident window.
sudo find /etc/kscore /var/lib/kscore /usr/bin/kscore-* \
    -newer "$EVIDENCE/00-observations.txt" -ls

# auditd records for the suspect window (if installed).
sudo ausearch -ts "06:00:00" -te "07:00:00" -m USER_CMD,EXECVE

# Established outbound connections (look for unexpected
# destinations).
sudo ss -tnp state established \
    | grep -vE ':(22|80|443|4222|5397|8080|8081)\b'

# Recent logins / sudo invocations.
sudo journalctl _COMM=sshd  --since "24 hours ago" | grep -i accepted
sudo journalctl _COMM=sudo  --since "24 hours ago" | grep -i COMMAND
```

---

## Phase 3 — Containment and recovery

The exact recovery steps depend on what was compromised. Apply only
the ones that match your confirmed exposure.

### 3.1 Revoke a compromised API key

```bash
# Get the key's id (NOT the cleartext -- the cleartext is gone
# the moment it was issued).
curl -sS -H "Authorization: Bearer $API_KEY" \
    http://127.0.0.1:8080/api/v1/apikeys | jq '.apikeys[] | {id, name, role}'

# Delete it. Auth chain (API key -> RBAC -> rate-limit) re-verifies
# on every request, so the next request from a deleted key fails
# closed without a server restart.
curl -sS -X DELETE \
    -H "Authorization: Bearer $API_KEY" \
    "http://127.0.0.1:8080/api/v1/apikeys/<id-to-revoke>"

# Verify by attempting a call with the revoked key -- expect 401.
curl -i -H "Authorization: Bearer <revoked-key>" \
    http://127.0.0.1:8080/api/v1/apikeys
```

### 3.2 Revoke a compromised identity token (SPIFFE/JWT)

```bash
# Revoke by token id.
kscorectl identity token revoke <token-id>

# Cleanup the revoked / expired token registry (housekeeping).
kscorectl identity token cleanup
```

### 3.3 Rotate the signing CA

Use when the intermediate / signing-CA private key was exposed, or
when you cannot enumerate every issued identity and need a clean
break.

```bash
# Rotates the signing CA. Existing identities issued by the
# previous signing CA become untrusted; agents must re-bootstrap
# under the new CA before they can reconnect. The root CA is
# NOT rotated by this command -- root rotation is operator-driven
# manual ceremony and is not in v0.1 scope.
kscorectl identity ca rotate-signing

# Verify the new signing-CA fingerprint surfaced.
kscorectl identity ca info -o json | jq '.signing_ca.fingerprint'
```

Plan for the bootstrap-chain disruption — every agent will need a
fresh enrollment token from the operator-side bootstrap path before
its next connect.

### 3.4 Rotate the HMAC secret

Use when the HMAC secret in `/etc/kscore/server.yaml` was exposed
(e.g., the config was leaked, or the file was world-readable for a
window).

This invalidates EVERY agent's bootstrap PSK. Plan agent
re-enrollment before doing this in production. The mechanics are
identical to Scenario C in
[`emergency-rollback.md`](emergency-rollback.md) — config edit +
restart:

```bash
sudo cp /etc/kscore/server.yaml /etc/kscore/server.yaml.pre-rotation
HMAC=$(openssl rand -hex 32)
sudo sed -i "s|^\(\s*hmacsecret:\s*\"\).*\"|\1${HMAC}\"|" /etc/kscore/server.yaml
sudo systemctl restart kscore-server
```

### 3.5 Restore from a pre-incident snapshot

Use when the state DB, JetStream store, or config has been mutated
in a way you cannot roll back surgically. Same procedure as Scenario
D in [`emergency-rollback.md`](emergency-rollback.md); summarised
here:

```bash
# 1. List snapshots taken before the incident window.
kscore-cluster-backup list /var/backups/kscore/

# 2. Verify the snapshot integrity before restoring.
kscore-cluster-backup verify --input /var/backups/kscore/<snap>.tar

# 3. Stop the server.
sudo systemctl stop kscore-server

# 4. Restore. --force is required to overwrite a populated cluster.
sudo kscore-cluster-backup restore \
    --input /var/backups/kscore/<pre-incident>.tar \
    --force

# 5. Restart and verify.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

Without a pre-incident backup, the only path is forward-fix: rotate
credentials, audit declared state, and rebuild trust manually. The
backup-creation runbook is
[`docs/runbooks/backup-restore.md`](backup-restore.md).

### 3.6 Rebuild a compromised host

For a host you cannot verify is clean: reprovision from a known-good
OS image, reinstall the kscore packages from the official artifact
location (whichever package source your deployment uses; the public
package repo is a v1.0 release-ceremony deliverable), restore the
host's role from its configuration source of truth, then re-enroll
the agent under a fresh bootstrap token.

```bash
# On the rebuilt host, after package install:
sudo $EDITOR /etc/kscore/agent.yaml   # set agent.id + nats.urls
sudo systemctl start kscore-agent

# From the operator workstation: confirm the agent reconnected.
kscorectl events list --type agent_connected --since 5m
```

---

## Phase 4 — Verification

After containment and recovery, confirm the system is in a
trustworthy state:

- [ ] `curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'`
      returns `ready=true` with every component reporting `status=ok`
- [ ] `kscorectl audit log --limit 5` shows the verification queries
      themselves landing — proves the audit pipeline is healthy and
      the read path works
- [ ] `kscorectl events list --type agent_connected --since 30m`
      shows every expected agent reconnected
- [ ] `kscorectl identity ca info` shows the expected signing-CA
      fingerprint (rotated, if rotation was part of containment)
- [ ] `kscorectl identity token list --unused` is empty or matches
      the expected post-rebuild enrollment-token inventory
- [ ] `curl -sS -H "Authorization: Bearer $API_KEY"
      http://127.0.0.1:8080/api/v1/apikeys` shows only the API keys
      you intend to exist
- [ ] No process on any host is listening on a port that wasn't
      listening before the incident (`ss -tlnp`)
- [ ] Binary hashes match the package manager's expected hashes
      (`dpkg --verify kscore-server kscore-cli kscore-agent` or
      `rpm -V`)
- [ ] No alerts firing in your Prometheus / observability stack for
      `kscore_health_ready{component=...} == 0` or
      sustained `rate(kscore_api_errors_total[5m])` regression

---

## Phase 5 — Post-incident

### 5.1 Collect the incident bundle

```bash
D=/tmp/incident-bundle-$(date -u +%Y%m%dT%H%M%S)
mkdir -p "$D"

# All evidence collected during the incident.
sudo cp -r "$(dirname "$EVIDENCE")" "$D/evidence/"

# Redact known-sensitive material from configs (best-effort --
# INSPECT BEFORE SHARING).
for f in "$D"/evidence/*/server.yaml.at-incident "$D"/evidence/*/agent.yaml.at-incident; do
    [ -f "$f" ] || continue
    sudo sed -i 's/^\(\s*hmacsecret:\s*"\).*"/\1REDACTED"/' "$f"
    sudo sed -i 's/^\(\s*master_key:\s*\).*$/\1REDACTED/'    "$f"
done

# Timeline (TSV of audit events in time order).
cp "$EVIDENCE/audit-timeline.tsv" "$D/timeline.tsv" 2>/dev/null || true

# Versions.
sudo dpkg -l kscore-server kscore-cli kscore-agent > "$D/installed.txt" 2>/dev/null || \
    sudo rpm -q kscore-server kscore-cli kscore-agent > "$D/installed.txt"
/usr/bin/kscore-server --version >> "$D/installed.txt"

# Health snapshot post-recovery.
curl -fsS http://127.0.0.1:8080/health/ready > "$D/health-ready.json" 2>/dev/null || true

# Bundle.
sudo tar czf "${D}.tar.gz" -C /tmp "$(basename "$D")"
echo "Incident bundle: ${D}.tar.gz"
```

Inspect the bundle before sharing. The redaction is best-effort and
will miss operator-added secrets in unexpected keys.

### 5.2 Write the incident report

Use whichever template your org runs. The minimum is:

- Detection time, response time, containment time, recovery time
- Severity at detection vs final severity, with justification
- Initial scope vs final scope (what turned out to be in / out)
- Confirmed compromise (specific accounts / hosts / data)
- Root cause (single sentence + supporting evidence references
  back into the audit / events bundle)
- Containment + recovery actions taken (each tied to a phase in
  this runbook)
- Outstanding remediation work and ownership

### 5.3 If external coordination is required

If the incident involves a security bug in keystone-core itself (as
opposed to operator misconfiguration or attacker exploitation of an
intended feature), file a security advisory per the disclosure flow
in [`SECURITY.md`](../../SECURITY.md). Do NOT open a public issue —
the private-disclosure path exists precisely so the fix lands
before the exploit detail does.

The keystone-core.io security mailbox referenced in
[`SECURITY.md`](../../SECURITY.md) is being provisioned (currently
on the [`.lychee.toml`](../../.lychee.toml) exclude list for that
reason). Until provisioning completes, use the maintainer contact
in [`OWNERSHIP.md`](../../OWNERSHIP.md).

### 5.4 Post-mortem

Run the post-mortem with the standard blameless framing. Specific
checks to run during the meeting:

- Was the detection signal something the events stream could have
  alerted on? If yes, add the alert.
- Was the containment slowed by a missing v0.1 capability? If yes,
  file a [`docs/project/ROADMAP.md`](../project/ROADMAP.md) entry
  so the gap is tracked, not lost.
- Were any commands in this runbook wrong, stale, or missing? Patch
  the runbook in the same week as the incident — runbooks rot
  fastest right after they're used.

---

## Appendix A — Indicator-of-compromise quick queries

```bash
# Audit: bulk secret reads from a single principal.
kscorectl audit log --action "secret.read" --since 24h -o json \
    | jq -r '.[] | .user' | sort | uniq -c | sort -rn | head -10

# Audit: any policy delete attempts (policy CRUD is Unimplemented
# in v0.1 -- successful policy deletes should NOT exist; any audit
# row claiming one is a forensic artifact worth investigating).
kscorectl audit log --action "policy.delete" --since 30d

# Events: agent disconnects clustered in time (possible mass
# eviction or NATS broker disruption).
kscorectl events list --type agent_disconnected --since 1h -o json \
    | jq -r '.[] | .timestamp' | cut -c1-16 | sort | uniq -c

# Filesystem: kscore binaries modified outside the package
# manager's purview.
sudo find /usr/bin -name 'kscore-*' -newer /var/lib/dpkg/info/kscore-server.md5sums \
    -ls 2>/dev/null

# Network: long-lived connections to non-standard ports.
sudo ss -tnp state established \
    | grep -vE ':(22|80|443|4222|5397|8080|8081|6060)\b'

# Auth: ssh accepted-publickey events in the suspect window.
sudo journalctl _COMM=sshd --since "24 hours ago" \
    | grep -i 'Accepted publickey'
```

## Appendix B — Evidence retention guidance

Specific retention is a per-organisation policy question; the v0.1
runbook only documents the technical storage footprint.

| Evidence type            | Typical bytes | Notes |
|--------------------------|---------------|-------|
| Audit-log JSONL export   | ~1-10 MB/day  | Compresses well; `gzip` before archival |
| Events JSONL export      | ~10-50 MB/day | Compresses well |
| State DB (SQLite) copy   | ~10 MB - 1 GB | Depends on declared state volume |
| JetStream store tarball  | ~50 MB - 5 GB | Largest single artifact; sample / prune before long-term retention |
| journalctl 24h capture   | ~50-500 MB    | Per-host |
| Process / socket tables  | < 1 MB        | Captured per-host at incident start |

Encrypt and offsite anything that's leaving the trust boundary;
treat the incident bundle as it would be treated if it were the
attacker's exfiltration target.

## Related runbooks

- [`emergency-rollback.md`](emergency-rollback.md) — scenarios for
  rolling back a bad change (Scenario C: config revert / HMAC
  rotation; Scenario D: catastrophic restore from snapshot)
- [`upgrade-cluster.md`](upgrade-cluster.md) — the change-management
  path that should be paired with pre-change snapshots
- [`troubleshooting.md`](troubleshooting.md) — symptom-to-diagnosis
  for the non-security failure modes
- [`backup-restore.md`](backup-restore.md) — creating the snapshots
  that make Phase 3.5 (restore-from-snapshot) possible

## Escalation

- **Operational incident response** procedures (internal to the
  team running keystone-core): see
  [`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).
- **Reporting a security bug in keystone-core itself**: private
  disclosure flow in [`SECURITY.md`](../../SECURITY.md). Do NOT
  open a public issue. The security mailbox is pre-provisioning;
  until it is live, route through the maintainer contact in
  [`OWNERSHIP.md`](../../OWNERSHIP.md).
- **Project ownership + maintainer contact**:
  [`OWNERSHIP.md`](../../OWNERSHIP.md).
