# C04-I acceptance evidence

## Result

C04-I implements the Linux local-operator substrate frozen by C04-A. The
production server owns `/run/keystone/operator.sock`, derives the peer uid from
`SO_PEERCRED`, resolves current membership before reading any request byte, and
persists an in-band denial before writing its generic response. No journey verb,
NATS connection, or agent transport is introduced.

The configured defaults are 32 connections awaiting authorization, a 5 second
authorization timeout, 64 authorized connections, and a 64 KiB frame. These
bound a local administrative surface without implying a deployment load
profile; C15 revisits them under load. The in-flight-request limit remains
registered to C05 because C04 has no operation that can be in flight.

## Membership source and failure behavior

Each connection uses `os/user.LookupId` followed by `GroupIds`, without a
membership cache. The shipped `CGO_ENABLED=0` Linux image therefore reads the
current passwd/group files; C04 does not claim support for a network NSS
directory. A bounded worker set performs lookup. Lookup failure fails closed;
a lookup or configured pre-decision fault that exceeds the authorization
timeout closes unanswered and releases the unauthorized-connection slot.
Workers are bounded even if the underlying lookup does not return.

## Contract result

The unmodified production server passed all 25 C04 cases:

```text
KEYSTONE_REQUIRE_DOCKER=1 go test -count=1 -tags contract ./test/contract/operator
ok go.keystone-core.io/keystone-core/test/contract/operator 412.470s
exit 0
```

Docker was not installed in the development environment. The command used a
temporary Docker-compatible rootless Podman wrapper and the repository's
unchanged contract image and production binary. These container results are
pull-request feedback only. C13's VM harness remains the acceptance gate for
real users, supplementary groups, filesystem policy and `SO_PEERCRED`.

The four C05 entries (`CLI-1`–`CLI-3`, `LIM-5`) remain deferred. C04 adds no
requirement-register row under `D-C04-2`; the register gap remains explicit.

## Planted defects

Every frozen case failed against production code with a defect in the behavior
it observes. Each command below returned exit 1; the unmodified command above
then returned exit 0.

| Planted defect | Cases rejected |
|---|---|
| Directory `0755`, socket `0666` | `SOCK-1`, `SOCK-3`, `SOCK-4`, `DENY-5` |
| Missing admin group defaults to `root` | `SOCK-2` |
| In-band membership check authorizes every connected peer | `ORD-1`–`ORD-3`, `DENY-1`–`DENY-4`, `DENY-6`, `DENY-7` |
| A denied connection is read before its response | `ORD-1` |
| Denial response precedes its record by 20 ms | `ORD-2` |
| A denial is answered after its record write fails | `ORD-3` |
| Denial adds a `reason` member | `DENY-3` |
| Denial records uid/name `0/root` | `DENY-6`, `DENY-7` |
| Every existing socket-path object is removed | `REC-1`–`REC-3` |
| Stale sockets are never recovered | `REC-4` |
| Group-writable socket directory is trusted | `REC-5` |
| Configured pre-authorization limit, timeout and authorized limit are ignored | `LIM-1`–`LIM-3` |
| Hostile frame length is allocated before comparison | `LIM-4` |
| Server opens a TCP listener | `ISO-1` |
| Enabled fault configuration is ignored | `FLT-1` |

Representative failure output included the consumed-byte clean EOF for
`ORD-1`, zero surviving records after `ORD-2`'s SIGKILL, a denial frame despite
`ORD-3`'s full filesystem, 4,199,168 KiB of virtual-size growth for `LIM-4`,
and an observed TCP socket for `ISO-1`.

The local `make check` run passed every gate through all four contract packages,
including authorization, operator, persistence and protocol. Its final
`container-suite` could not run because this host has neither Docker nor a
Podman Compose provider. The branch-push CI runner supplies Docker and is the
complete `make check` result; no local full-green claim is made.

## Store and compatibility

Server migration 3 replaces the original audit shape transactionally, copies
existing rows while retaining the nullable general-purpose actor field, permits
a connection-level record with null job and target, and adds authoritative uid
and username-snapshot columns for local operators. C02's executable contract
now requires migrations 1 and 2 as the ordered prefix it owns rather than
claiming no later migration can exist. Its frozen identifier and requirement
are unchanged.

## Risk disposition

`RSK-8` remains accepted by the project maintainer. C04 implements and exercises
kernel-derived attribution and current-membership denial, but those controls
disclose activity and do not distinguish an operator from malware in that
operator's session. It is renewed to 2027-03-13 or C13, where the real-host VM
gate can validate the implemented boundary.
