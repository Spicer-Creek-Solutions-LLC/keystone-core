# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Short-Term Priority (1-2 Releases)

### Critical (Blocks Functionality)

1. ~~**Implement `GetCurrentVersion()` in upgrade package**~~ ✅ COMPLETE
   - Implemented via component inspector system in `pkg/upgrade/inspector.go`
   - SelfInspector for server/agent, NATSInspector, DatabaseInspector, EtcdInspector, BinaryInspector
   - HTTPVersionProvider now uses inspector registry for version detection

2. ~~**Fix semver comparison in module lockfile**~~ ✅ COMPLETE
   - Created new `pkg/semver` library with rich version comparison capabilities
   - Features: Version parsing, Diff with change classification (major/minor/patch), Constraints (^, ~, ranges), Sorting
   - `GetUpdateType()` now correctly identifies major, minor, patch, and prerelease changes
   - Library is reusable by external projects

3. ~~**Consolidate signing packages**~~ ✅ COMPLETE
   - Created shared `pkg/signing/` package for all signing operations
   - Key-based signing: RSA, ECDSA, Ed25519 with PKCS8/PKIX format
   - Keyless signing: Sigstore/Fulcio for CI/CD (pre-provided OIDC tokens)
   - Updated `pkg/module/verify` and `pkg/blueprint/registry` to use shared package
   - Interactive OIDC flow (browser-based) intentionally deferred to future epic

### High Priority (Feature Gaps)

4. ~~**Implement network configuration apply operations**~~ ✅ COMPLETE
   - Implemented apply operations for ifupdown, systemd-networkd, and netplan
   - Static IP and DHCP support for all three backends
   - ifupdown: Parses/updates `/etc/network/interfaces`, activates via `ifup/ifdown`
   - systemd-networkd: Generates `.network` files in `/etc/systemd/network/`, reloads via `networkctl`
   - netplan: Generates YAML in `/etc/netplan/`, applies via `netplan apply`
   - Automatic backup of existing configuration before changes

5. **Add package-level documentation**
   - Packages missing docs: `pkg/api/`, `pkg/cli/`, `pkg/gitops/`, `pkg/module/`, `pkg/proto/`, `pkg/testing/`
   - Impact: Developer onboarding and code understanding hampered
   - Needs: Package doc comments explaining purpose and usage

6. **SNMP INFORM operation workaround**
   - Location: `pkg/protocols/snmp/v2c.go:300`
   - Issue: gosnmp library doesn't support INFORM in v1.35.0
   - Impact: Cannot send SNMP INFORM notifications
   - Needs: Custom implementation or library upgrade when available

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
