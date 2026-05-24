#!/usr/bin/env bash
# Unit-style test for the kscore-server and kscore-agent postinst
# scripts. Runs the scripts in a fakeroot-style sandbox (PREFIX
# substitution + skipped systemctl/useradd) and asserts that:
#   - template substitution produces a valid /etc/kscore/server.yaml
#     with an HMAC secret that is exactly 64 hex chars
#   - the script is idempotent (re-running leaves the rendered
#     config alone)
#   - the agent postinst's template install copies bytes verbatim
#
# We do NOT exercise the real useradd / systemctl paths here because
# they require root + a live systemd; those are covered by the
# Phase D VM smoke. This test stays runnable on any developer box
# without privileges.

set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# -- helpers --------------------------------------------------------------

# Build a wrapper that re-points the postinst at $WORK by substituting
# every absolute path the script touches. We override:
#   - /etc/kscore           -> $WORK/etc/kscore
#   - /var/lib/kscore       -> $WORK/var/lib/kscore
#   - /var/log/kscore       -> $WORK/var/log/kscore
#   - /run/kscore           -> $WORK/run/kscore
#   - /usr/share/kscore/... -> $WORK/usr/share/kscore/...
# And we mask out the user/group + systemd steps via PATH stubs.
rewrite() {
    local src=$1 dst=$2
    sed -e "s|=/etc/kscore|=${WORK}/etc/kscore|g" \
        -e "s|=/var/lib/kscore|=${WORK}/var/lib/kscore|g" \
        -e "s|=/var/log/kscore|=${WORK}/var/log/kscore|g" \
        -e "s|=/run/kscore|=${WORK}/run/kscore|g" \
        -e "s|=/usr/share/kscore|=${WORK}/usr/share/kscore|g" \
        "$src" >"$dst"
    chmod +x "$dst"
}

# Stub `getent`, `useradd`, `groupadd`, `userdel`, `groupdel`,
# `systemctl` — let the script believe everything succeeded. We
# also stub `install` so unprivileged callers can write to $WORK
# without -o root failing.
make_stubs() {
    local bin=$WORK/bin
    mkdir -p "$bin"
    cat >"$bin/getent" <<'EOF'
#!/bin/sh
exit 2
EOF
    cat >"$bin/useradd" <<'EOF'
#!/bin/sh
exit 0
EOF
    cat >"$bin/groupadd" <<'EOF'
#!/bin/sh
exit 0
EOF
    cat >"$bin/userdel" <<'EOF'
#!/bin/sh
exit 0
EOF
    cat >"$bin/groupdel" <<'EOF'
#!/bin/sh
exit 0
EOF
    cat >"$bin/systemctl" <<'EOF'
#!/bin/sh
exit 0
EOF
    # chown: skip — the unprivileged test can't reassign ownership,
    # and the kscore user/group don't exist in the sandbox anyway.
    cat >"$bin/chown" <<'EOF'
#!/bin/sh
exit 0
EOF
    # install: drop the -o/-g flags (which fail without root) and
    # delegate the rest to /usr/bin/install.
    cat >"$bin/install" <<'EOF'
#!/bin/sh
args=""
while [ $# -gt 0 ]; do
    case "$1" in
        -o|-g) shift 2 ;;
        *) args="$args $1"; shift ;;
    esac
done
exec /usr/bin/install $args
EOF
    chmod +x "$bin"/*
}

# -- setup ----------------------------------------------------------------

mkdir -p \
    "$WORK/etc/kscore" \
    "$WORK/var/lib/kscore" \
    "$WORK/var/log/kscore" \
    "$WORK/run/kscore" \
    "$WORK/usr/share/kscore"

cp "$REPO_ROOT/deploy/config/server.yaml.template" "$WORK/usr/share/kscore/server.yaml.template"
cp "$REPO_ROOT/deploy/config/agent.yaml.template"  "$WORK/usr/share/kscore/agent.yaml.template"

make_stubs

REWRITTEN_SERVER=$WORK/bin/server.postinst
REWRITTEN_AGENT=$WORK/bin/agent.postinst
rewrite "$REPO_ROOT/deploy/packaging/kscore-server.postinst" "$REWRITTEN_SERVER"
rewrite "$REPO_ROOT/deploy/packaging/kscore-agent.postinst"  "$REWRITTEN_AGENT"

# Stop the script from looking at the real /run/systemd/system.
SYSTEMD_HIDE=$WORK/sysd-no-such-dir
sed -i "s|/run/systemd/system|${SYSTEMD_HIDE}|g" "$REWRITTEN_SERVER" "$REWRITTEN_AGENT"

# -- server postinst ------------------------------------------------------

echo "test: kscore-server.postinst — first install renders config"
PATH="$WORK/bin:$PATH" "$REWRITTEN_SERVER"

[ -f "$WORK/etc/kscore/server.yaml" ] || fail "server.yaml not created"

if grep -q "__HMAC_SECRET__" "$WORK/etc/kscore/server.yaml"; then
    fail "server.yaml still contains __HMAC_SECRET__ placeholder"
fi

# Extract the rendered hmacsecret value and assert it's 64 hex chars.
HMAC=$(grep -E '^\s*hmacsecret:' "$WORK/etc/kscore/server.yaml" | awk -F'"' '{print $2}')
if ! echo "$HMAC" | grep -qE '^[0-9a-f]{64}$'; then
    fail "rendered hmacsecret is not 64 hex chars (got: '$HMAC')"
fi
echo "  ok: server.yaml present, hmacsecret=64 hex chars"

echo "test: kscore-server.postinst — idempotency (re-run leaves config alone)"
SUM_BEFORE=$(sha256sum "$WORK/etc/kscore/server.yaml" | awk '{print $1}')
PATH="$WORK/bin:$PATH" "$REWRITTEN_SERVER"
SUM_AFTER=$(sha256sum "$WORK/etc/kscore/server.yaml" | awk '{print $1}')
if [ "$SUM_BEFORE" != "$SUM_AFTER" ]; then
    fail "second postinst run modified the config (hmacsecret regenerated?)"
fi
echo "  ok: config unchanged on re-run"

# -- agent postinst -------------------------------------------------------

echo "test: kscore-agent.postinst — first install copies template"
mkdir -p "$WORK/var/lib/kscore-agent" "$WORK/var/log/kscore-agent"
sed -i "s|=/var/lib/kscore-agent|=${WORK}/var/lib/kscore-agent|g; s|=/var/log/kscore-agent|=${WORK}/var/log/kscore-agent|g" "$REWRITTEN_AGENT"
PATH="$WORK/bin:$PATH" "$REWRITTEN_AGENT"

[ -f "$WORK/etc/kscore/agent.yaml" ] || fail "agent.yaml not created"
if ! diff -q "$WORK/usr/share/kscore/agent.yaml.template" "$WORK/etc/kscore/agent.yaml" >/dev/null; then
    fail "agent.yaml differs from template — postinst should be a verbatim copy"
fi
echo "  ok: agent.yaml installed verbatim from template"

echo "test: kscore-agent.postinst — idempotency"
sha_a_before=$(sha256sum "$WORK/etc/kscore/agent.yaml" | awk '{print $1}')
PATH="$WORK/bin:$PATH" "$REWRITTEN_AGENT"
sha_a_after=$(sha256sum "$WORK/etc/kscore/agent.yaml" | awk '{print $1}')
if [ "$sha_a_before" != "$sha_a_after" ]; then
    fail "second agent postinst run modified agent.yaml"
fi
echo "  ok: agent.yaml unchanged on re-run"

echo
echo "PASS: all postinst tests"
