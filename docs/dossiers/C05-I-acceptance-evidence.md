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
