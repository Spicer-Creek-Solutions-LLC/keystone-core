#!/bin/bash
# Smoke test runner for Keystone Core
# Runs quick smoke tests to verify basic functionality before commits
#
# Usage:
#   ./scripts/smoke-test.sh           # Run all smoke tests
#   ./scripts/smoke-test.sh database  # Run database smoke tests only
#   ./scripts/smoke-test.sh quick     # Run minimal tests (SQLite only)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default test timeout
TIMEOUT="${SMOKE_TEST_TIMEOUT:-60s}"

print_status() {
    echo -e "${GREEN}[SMOKE]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[SMOKE]${NC} $1"
}

print_error() {
    echo -e "${RED}[SMOKE]${NC} $1"
}

run_database_smoke_tests() {
    print_status "Running database smoke tests..."

    cd "$PROJECT_ROOT"

    # Always run SQLite tests (no external dependencies)
    print_status "Testing SQLite backend..."
    if go test -v -timeout="$TIMEOUT" -run "TestSQLite" ./test/smoke/... ; then
        print_status "SQLite smoke tests passed"
    else
        print_error "SQLite smoke tests failed"
        return 1
    fi

    # Try PostgreSQL tests if available
    if [ -n "$KSCORE_TEST_POSTGRES_DSN" ] || nc -z localhost 5432 2>/dev/null; then
        print_status "Testing PostgreSQL backend..."
        if go test -v -timeout="$TIMEOUT" -run "TestPostgreSQL" ./test/smoke/... ; then
            print_status "PostgreSQL smoke tests passed"
        else
            print_warning "PostgreSQL smoke tests failed (non-blocking)"
        fi
    else
        print_warning "PostgreSQL not available, skipping PostgreSQL tests"
    fi

    # Run compatibility tests
    print_status "Testing database backend compatibility..."
    if go test -v -timeout="$TIMEOUT" -run "TestDatabaseBackendCompatibility" ./test/smoke/... ; then
        print_status "Compatibility tests passed"
    else
        print_error "Compatibility tests failed"
        return 1
    fi

    return 0
}

run_quick_smoke_tests() {
    print_status "Running quick smoke tests (SQLite only)..."

    cd "$PROJECT_ROOT"

    # SQLite-only tests for speed
    if go test -v -timeout=30s -run "TestSQLiteSmokeTest" ./test/smoke/... ; then
        print_status "Quick smoke tests passed"
        return 0
    else
        print_error "Quick smoke tests failed"
        return 1
    fi
}

run_compilation_check() {
    print_status "Checking compilation..."

    cd "$PROJECT_ROOT"

    # Check main packages compile
    if go build -o /dev/null ./cmd/... 2>/dev/null; then
        print_status "Compilation check passed"
        return 0
    else
        print_error "Compilation check failed"
        return 1
    fi
}

run_all_smoke_tests() {
    print_status "Running all smoke tests..."

    local failed=0

    # Compilation check
    if ! run_compilation_check; then
        failed=1
    fi

    # Database tests
    if ! run_database_smoke_tests; then
        failed=1
    fi

    return $failed
}

# Parse command line arguments
case "${1:-all}" in
    database|db)
        run_database_smoke_tests
        ;;
    quick|fast)
        run_quick_smoke_tests
        ;;
    compile|build)
        run_compilation_check
        ;;
    all)
        run_all_smoke_tests
        ;;
    help|-h|--help)
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  all       Run all smoke tests (default)"
        echo "  database  Run database smoke tests only"
        echo "  quick     Run minimal tests (SQLite only)"
        echo "  compile   Check compilation only"
        echo "  help      Show this help"
        echo ""
        echo "Environment variables:"
        echo "  KSCORE_TEST_POSTGRES_DSN   PostgreSQL connection string"
        echo "  SMOKE_TEST_TIMEOUT         Test timeout (default: 60s)"
        exit 0
        ;;
    *)
        print_error "Unknown command: $1"
        echo "Run '$0 help' for usage"
        exit 1
        ;;
esac

exit_code=$?

if [ $exit_code -eq 0 ]; then
    print_status "All smoke tests passed!"
else
    print_error "Some smoke tests failed"
fi

exit $exit_code
