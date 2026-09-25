# C05-I acceptance evidence

C05-I implements the enrollment behaviour C05-A froze, in six stages under one
approved plan. Each stage removes its cases from
`test/contract/enrollment/pending-requirements.json` only after every one has
failed against a defect planted in production code. The C05-A amendment the
plan depended on is issue #384, merged as pull request #385.

This file grows one section per stage. C05 is complete when the last stage
records the epic tick.

## Stage I1 — issuance, with no broker

### Result

`keystone enroll create --agent-name <name> [--ttl-seconds <n>]` asks the server
over ADR-0009's socket for one token and prints its version-1 bundle once, to
standard output and nowhere else. The server assigns a 128-bit agent identifier
and a 256-bit token identifier from fresh randomness, mints the bootstrap
identity through the same `natsauth.Minter` the generator uses, and records the
token and its issuance audit in one transaction before it answers.

The server reads `[nats]` and `[service]` at startup and refuses to start
unless every configured input is correct: each credential in its own slot, a
signing seed that issued both, a CA that parses, a service signing private-key
file with no group or other permission bit, and two keys that parse. It opens no
credential it was not configured with.

The operator socket answers one request at a time. It does not read a
connection's next frame until it has written the current response, so replies
are in request order.

Cases implemented: `ISS-1`, `CLI-1`, `CLI-2`, `CLI-3`, `LIM-5`, `FLT-2`,
`JWT-1`, `BND-1`, `SKEY-1`, `ROLE-1`.

**`ID-1` moves to I2.** Its requirement has two halves, and the second — *"never
an enrolling-party value"* — needs an enrolling party. I1 has none.

### Contract result

The unmodified production binaries passed every I1 case, inside a full
`make check` with every file staged:

```text
contract: test/contract/enrollment
ok  go.keystone-core.io/keystone-core/test/contract/enrollment  19.460s
contract: test/contract/operator
ok  go.keystone-core.io/keystone-core/test/contract/operator  28.040s
pending-contract: 29 registered case(s) across 4 package(s) fail for their documented reasons
contract-immutability-check: test/contract/enrollment matches ae80fb12e7d86d40ff1318f431449393e5b7c729 (3 declared amendment(s) since 6115c70f35fb0aa03f6642a1de36471976650f05)
check: ok
```

The 29 pending cases are the enrollment contract's remaining ones; C04's
operator contract, which starts servers with no enrollment configured, still
passes in full. The immutability line is the proof that I1 changed no frozen
file.

The cases run in a container with no network, against the production
`keystone-server` and `keystone`, with the generator's real deployment and
freshly generated service keys provisioned into it. Accounts, modes and
`SO_PEERCRED` in a container are pull-request feedback, not acceptance
(`ADR-0010` § 1).

### Planted defects

Each defect below was planted in production code, the named case was run
against it, and the case failed. The unmodified code then passed.

| Planted defect | Case rejected |
|---|---|
| The token is recorded 200 ms after the response, not before | `ISS-1` |
| The CLI also writes the bundle to standard error | `ISS-1` |
| The server logs the bootstrap credential | `ISS-1` |
| The CLI says which layer refused (`socket permissions`) | `CLI-1` |
| An in-band denial is a server error, exit `1` | `CLI-2` |
| A missing socket is reported as unauthorized, exit `10` | `CLI-3` |
| The server reads the next frame's prefix before writing its reply | `LIM-5` |
| Requests on one connection are served concurrently | `LIM-5` |
| The response hold is enabled without its audit record | `FLT-2` |
| The response hold is recorded when it is disabled | `FLT-2` |
| The bootstrap JWT may also subscribe to every token's reply | `JWT-1` |
| The bootstrap JWT outlives its token by a minute | `JWT-1` |
| The lifetime ceiling is 60 minutes, at the CLI and the server | `JWT-1` |
| The bundle carries a freshly generated, self-consistent signing half | `BND-1` |
| The bundle carries a freshly generated result half | `BND-1` |
| The signing-key file's mode is not checked | `SKEY-1` |
| The signing-key file must be exactly `0600` | `SKEY-1` |
| A service credential is accepted in any slot | `ROLE-1` |
| The server reads every file in the credential directory | `ROLE-1` |

**Two defects did not fail at first, and neither was accepted as a pass.**

- *The server logs the bootstrap credential* passed `ISS-1` on the first run.
  The case searched the host with a recursive `grep` and read its exit code, and
  `grep` exits `2` on the operator socket under `/run` **whatever it finds** — so
  the case could never report a leak. It now searches regular files only, reads
  the output rather than the exit code, and first proves the search finds a
  planted copy of the seed. Against the defect it names `/tmp/server.log`.
- *A service credential is accepted in any slot* passed `ROLE-1`, and the fault
  was the plant: `false && A || B` still evaluates `B`, so the permission
  comparison stayed in force. Planted as an early return that skips the check
  entirely, it is rejected.

### How the cases observe what they claim

- **`ISS-1` — durably.** A root process blocks on a FIFO; a member's probe
  requests a token and writes the FIFO the instant the response is complete;
  the server is SIGKILLed. Five rounds, and every token the server answered with
  is on record after its kill. "Nowhere else" is a search of every regular file
  on the host for the bootstrap seed, after a control proves it finds a planted
  copy.
- **`LIM-5` — not read.** The probe sends the first request, waits until
  `SIOCOUTQ` reports it consumed, then sends the second **one byte per
  `write()`**. Linux frees a sent buffer only when the peer has read all of it,
  so reading any byte of the second request lowers `SIOCOUTQ`. Each sample is
  taken before the socket is read: a server that writes its reply and then reads
  has put the reply in the probe's queue before the sample that shows the read,
  so a lowered sample with no complete reply behind it is a read of the second
  request before the first was answered. A two-second response hold keeps the
  first request in flight, and the case requires at least a hundred samples
  inside it.
- **`ROLE-1` — no unconfigured credential read.** `inotify` watches the
  credential directory from before the server starts, and must see the
  configured files opened — the control that the watch works — and none of the
  others.
- **`CLI-1` — indistinguishable.** Before comparing the CLI's outputs, the case
  proves which layer refused each principal: the outsider's `connect()` fails
  with `EACCES`, and root connects and receives `authorization_denied`. The two
  CLI runs must then agree in exit code, standard output and standard error.
- **`JWT-1` — C03's matrix.** The grants are compared with the bootstrap rows
  parsed from C03-A's frozen `contract_surface.go`, not with a restatement.

### Decisions made within the approved plan

- **Enrollment is optional.** A server with neither `[nats]` nor `[service]` is
  C04's operator substrate, and `enroll.create` is `unknown_operation` there. A
  server with part of them refuses to start. C04's contract starts servers with
  no broker configured and must keep passing; it does.
- **One error code is added: `invalid_request`,** for a known operation whose
  arguments are refused. C04 froze four codes and permitted extension; no frozen
  code fits a bad lifetime or name.
- **An issuance the server cannot complete is not answered.** A store or
  randomness failure closes the connection without a response, so the CLI never
  prints a token that is not on record. The CLI reports it and exits `1`.
- **Encodings, which `C05.md` § 3.3 gives C05 to decide.** Both service key
  files and both bundle halves are one line of standard base64: the signing key
  in `protocol.MarshalSigningKey`'s form, the public halves in their wire forms.
  `bootstrap_credentials` is the decorated credentials file a NATS client reads.
- **Every file path in the server's configuration must be absolute,** now
  including `store.path`, for the reason `config.ErrRelativePath` gives about the
  configuration file itself.
- **C05's one migration covers every stage.** Migration 4 replaces migration 1's
  unused `enrollment` table with ADR-0003's record — the token, its assigned
  identifier and label, the bootstrap key, the token state and its spent time,
  and the S1 public halves with the JWT minted for them — and adds the NATS key
  and label to `agents`. Nothing wrote the old table.

### What I1 does not show

- Modes, `fsync`, ownership and real accounts: C13's VM gate.
- Anything about the broker. I1 starts no broker; `EXP-1`, `OBS-1` and `TLS-1`
  are later stages.

## Stage I2 — the journey across the broker

### Result

`keystone-agent enroll --token-file <path|->` turns a bundle into a permanent
identity, and exits `0` only on S6's confirmation. The agent:

1. refuses a bundle file readable by group or other, before generating a key
   or sending anything;
2. writes its key artifact — NKey seed, `AST-4`, `AST-5` — atomically at `0600`
   before S1;
3. connects with the bootstrap credential, TLS-first and verifying the broker,
   and sends a request signed with its new key (S1);
4. verifies the service-signed reply against the bundle's half, writes the
   identity artifact atomically at `0600` and opens its ledger (S2);
5. connects with the permanent identity and publishes a signed presence (S3);
6. asks for confirmation until the service says active and spent (S6), then
   closes the bootstrap connection and removes the bundle.

The server runs the enrollment service on two TLS-first connections, one per
service credential. It checks every request in `ADR-0003` § 5's order, records
S1's halves and JWT once and returns the same JWT to the same halves, activates
the agent and spends the token in one write on the first fresh presence signed
with the recorded key (S4, S5), and confirms S6 on the spent token. It refuses
to start if it cannot reach and verify the broker.

Cases implemented: `STDIN-1`, `FILE-1`, `FILE-2`, `ARG-1`, `KEY-1`, `CRED-1`,
`TLS-1`, `NON-1`, `ISO-1`, and `ID-1` from I1.

**`LEDGER-1` moves to I4.** *"The ledger must exist before the server can reach
S4"* is an order between two hosts, and without a hold the harness can only race
it. The frozen `enrollment_agent_hold_after_identity_write_ms` hold, which I4
builds, stops the agent after S2 and before S3's proof; the ledger must exist at
that durable boundary. That is deterministic, and it is the observation the case
should rest on.

### The topology

The contract's fixture is now the topology `ADR-0010` § 2 designs, built with
the Docker CLI so each case owns its processes: a server network and **one
network per agent**, all internal, the pinned broker the only service on every
one, and no published port. The I1 cases run in it unchanged.
`TestTopologyMatchesCompose` holds it to `compose.yaml`'s shape.

`ISO-1` runs over **`compose.yaml` itself**: the server, both agents and the
broker come up from the file, the operator issues on the server's host, each
bundle is delivered out of band, and both agents enroll. This is the journey the
workflow's owed-gates list names for C05; I6 changes that entry.

**Two findings about the topology, both fixed here.**

- **Agents could reach each other.** `TESTING.md` and `ADR-0010` § 2 require
  agent-to-agent direct connections to fail, but both compose agents shared
  `agent-net`, and the isolation probe only tested separate networks. `ISO-1`
  freezes the requirement, so each agent now has a network of its own, and the
  broker is attached to all of them. Disabling inter-container traffic on one
  shared network would have cut the agents off from the broker as well.
- **The compose images could not build.** The e2e `Dockerfile` copied
  `go.mod` without `go.sum`, which fails once the server has any dependency.
  Nothing built those images — the gates parse the file and start only the
  broker — so the break was latent. `ISO-1` builds them. Each image now holds
  all three binaries, so the server's host has the operator CLI beside it, as
  `ADR-0009`'s local socket requires.

### Contract result

Inside a full `make check`, with every file staged:

```text
ok  go.keystone-core.io/keystone-core/test/contract/enrollment  95.672s
ok  go.keystone-core.io/keystone-core/test/contract/operator  43.163s
pending-contract: 19 registered case(s) across 4 package(s) fail for their documented reasons
contract-immutability-check: test/contract/enrollment matches ae80fb12e7d86d40ff1318f431449393e5b7c729 (3 declared amendment(s) since 6115c70f35fb0aa03f6642a1de36471976650f05)
check: ok
```

The first full run failed on the fixture, not the product: Docker had run out of
address pools. The cleanup removed each agent's network while the broker was
still attached to it, the removal failed silently, and every case leaked a
network. Networks are now removed after every container, and a failed removal
fails the case.

### Planted defects

Each was planted in production code — or, for `ISO-1`, in `compose.yaml` — and
the named case failed against it for the reason it states.

| Planted defect | Case rejected |
|---|---|
| `--token-file -` is read as a path | `STDIN-1` |
| A bundle read from standard input is spooled to disk | `STDIN-1` |
| The bundle file's mode is not checked | `FILE-1` |
| The mode is checked only after keys are generated | `FILE-1` |
| The bundle is kept after exit `0` | `FILE-2` |
| The agent runs a child process with the bootstrap seed in its argv | `ARG-1` |
| The agent puts its NKey seed in its NATS client name | `KEY-1` |
| The agent publishes its signing private key on its presence subject | `KEY-1` |
| The identity artifact is written in place | `CRED-1` |
| The identity artifact is mode `0644` | `CRED-1` |
| The identity artifact omits the service signing half | `CRED-1` |
| The agent skips TLS verification | `TLS-1` |
| The server verifies a fixed name instead of its configured one | `TLS-1` |
| Activation rewrites every agent's record | `NON-1` |
| A label that is a valid identifier becomes the identifier | `ID-1` |
| The service trusts the envelope's sender over its record | `ID-1` |
| The compose server is also on an agent's network | `ISO-1` |
| The compose agents share a network | `ISO-1` |

**One defect did not fail at first, and the fault was the plant.** The `ARG-1`
plant parsed the seed from the raw bundle, where the credential's newlines are
JSON escapes, so the parse failed and the child process never ran. Planted
through the bundle's parsed credential, the seed reaches argv and the case names
the process: `sleep 1 SUA…`.

### How the cases observe what they claim

- **`ARG-1`.** A sampler on each host reads every process's command line from
  `/proc` continuously while the operator issues and while the agent enrolls,
  and must itself have seen the command it was watching for — otherwise it
  proves nothing about the run.
- **`KEY-1`.** The agent's private keys are read from its artifact, in every
  representation — the NKey seed, both Keystone keys in base64 and raw. The
  server host's files and store, and the broker's `-DV` protocol trace, must hold
  none. The trace's control is that it does hold the request's public signing
  half.
- **`CRED-1`.** `inotify` on the agent's state directory: the identity
  artifact's name must appear only by `MOVED_TO`, never `CREATE`, `MODIFY` or
  `CLOSE_WRITE`, which a write in place would show part-written.
- **`TLS-1`.** Every connection is refused for a wrong name and for a CA that
  did not issue the broker's certificate — the server by refusing to start, the
  agent before sending anything — and succeeds when both are right. The broker
  accepts only TLS-first TLS 1.3 (G49), so every connection that succeeds is one.
- **`ID-1`.** The label is itself a valid 32-hex identifier, so a server that
  used it would pass every format check. The harness then connects with the
  token's own bootstrap credential and sends a correctly signed request naming
  itself as another agent: it is refused, and nothing is recorded.
- **`ISO-1`.** No shared network between any two of server, agent-1 and
  agent-2, and `ping` from each to the other's address fails, with the control
  that the server does reach the broker.

### Decisions made within the approved plan

- **A refused request gets a signed, bare denial.** `KindDenied` carries no
  reason, so expired, spent and invalid are indistinguishable (`ADR-0003` § 5),
  and the agent exits `10` at once rather than retrying until the token expires.
  The reason is in the audit record. The frozen reply shape is unchanged; the
  kind is an added value. **A denial is sent only once its record is durable**:
  if the audit write fails, the request is not answered, as C04's `ORD-3` holds
  the operator socket to. `TestDenialIsAnsweredOnlyOnceRecorded` forces the
  failure and fails against the version that answered anyway.
- **Presence's payload is `{}`.** Presence's content is C10's; S3 needs only a
  presence the service can verify.
- **An enrollment envelope's correlation identifier is its token.** It is the
  replay key `ADR-0005` § 6 gives both enrollment classes.
- **The server does not start without the broker.** It tries for 15 seconds,
  then exits `1`. A server that cannot enroll does not offer to issue tokens.
- **The compose server's admin group is Alpine's `wheel`,** which root is in, so
  the CLI runs as root on the server's host. The contract's own fixture keeps
  C04's separate member and non-member accounts.

### What I2 does not show

- Everything VM-bound, as for I1.
- Crash recovery and the fault points: I4 and I5. The agent already resumes
  from its artifacts, but no case yet stops it at a boundary.
