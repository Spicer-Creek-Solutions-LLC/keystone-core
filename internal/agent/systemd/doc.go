// SPDX-License-Identifier: Apache-2.0

// Package systemd renders + installs the keystone-core-agent
// systemd unit. Three operations are exposed:
//
//   - Render(params) → []byte: build the unit file.
//   - Install(ctx, params, opts): atomic-write to
//     /etc/systemd/system/keystone-core-agent.service, run
//     daemon-reload, optionally enable --now.
//   - Uninstall(ctx, opts): stop + disable + remove + daemon-reload.
//   - Status(ctx) (StatusResult, error): wraps systemctl
//     is-active + is-enabled.
//
// systemctl invocations go through the Runner interface so tests
// stub them. The default runner shells out via os/exec; tests
// drive a FakeRunner.
//
// v1.0 ships demo mode + a separate `kscore-agent service install`
// CLI step. Production-mode bootstrap auto-installs the unit (via
// Install here) when Epic 11/14/17 land and the demo-only gate
// lifts — see docs/project/ROADMAP.md.
//
// Linux-only. The Windows agent (post-v1.0) ships a service install
// via SCM; macOS (post-v1.0) via launchd. Both swap this package for a
// platform-specific equivalent.
package systemd
