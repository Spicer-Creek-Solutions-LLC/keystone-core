#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: scripts/setup-dev.sh [--install-tools] [--install-pre-commit] [--skip-deps]

Sets up a local development environment for Keystone Core.

Options:
  --install-tools       Install protobuf tools via make install-tools
  --install-pre-commit  Install git pre-commit hooks (requires pre-commit)
  --skip-deps           Skip go module download
EOF
}

install_tools=false
install_pre_commit=false
skip_deps=false

for arg in "$@"; do
  case "$arg" in
    --install-tools)
      install_tools=true
      ;;
    --install-pre-commit)
      install_pre_commit=true
      ;;
    --skip-deps)
      skip_deps=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage
      exit 1
      ;;
  esac
done

cd "$ROOT_DIR"

echo "==> Checking prerequisites"
for cmd in go git; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
done

echo "==> Go version"
go version

if ! $skip_deps; then
  echo "==> Downloading Go module dependencies"
  go mod download
else
  echo "==> Skipping dependency download"
fi

if $install_tools; then
  echo "==> Installing protobuf tools"
  make install-tools
fi

if $install_pre_commit; then
  if command -v pre-commit >/dev/null 2>&1; then
    echo "==> Installing pre-commit hooks"
    pre-commit install
  else
    echo "pre-commit not installed; install it and rerun with --install-pre-commit" >&2
  fi
fi

cat <<'EOF'

Setup complete.

Next steps:
  - Build: make build
  - Run tests (single package): timeout 60 go test ./pkg/<pkg>
  - Verify SDK examples: make sdk-verify
EOF
