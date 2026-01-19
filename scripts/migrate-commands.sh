#!/bin/bash
# migrate-commands.sh - Migrate scripts from deprecated kscorectl commands to new commands
#
# This script helps migrate shell scripts, CI/CD configs, and automation from
# deprecated kscorectl command patterns to the new restructured commands.
#
# Usage:
#   ./migrate-commands.sh <file>           # Show changes (dry run)
#   ./migrate-commands.sh --apply <file>   # Apply changes in place
#   ./migrate-commands.sh --dir <path>     # Scan directory for files to migrate
#
# Supported file types: .sh, .bash, .yml, .yaml, Makefile, Jenkinsfile, Dockerfile
#
# Epic 30 Command Migrations:
#   kscore-policy audit     -> kscore-audit log
#   kscore-policy report    -> kscore-audit report
#   kscore-gitops webhook   -> kscore-webhook
#   kscore-cluster backup   -> kscore-cluster-backup create
#   kscore-cluster restore  -> kscore-cluster-backup restore
#   kscore-files backend    -> kscore-files-storage backend
#   kscore-files mirrors    -> kscore-files-storage mirrors
#   kscore-identity federation -> kscore-federation
#   kscore-blueprint publish -> kscore-blueprint-publish publish
#   kscore-blueprint sign   -> kscore-blueprint-publish sign
#   kscore-blueprint verify -> kscore-blueprint-publish verify
#   kscore-blueprint rollback -> kscore-blueprint-state rollback
#   kscore-blueprint snapshot -> kscore-blueprint-state snapshot

set -euo pipefail

VERSION="1.0.0"
SCRIPT_NAME="$(basename "$0")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Disable colors if not a terminal
if [[ ! -t 1 ]]; then
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

usage() {
    cat << EOF
Usage: ${SCRIPT_NAME} [OPTIONS] <file|directory>

Migrate scripts from deprecated kscorectl commands to new commands.

Options:
    -a, --apply         Apply changes in place (creates .bak backup)
    -d, --dir           Scan directory recursively
    -n, --dry-run       Show changes without applying (default)
    -q, --quiet         Suppress informational messages
    -v, --verbose       Show detailed information
    --no-backup         Don't create backup files when applying
    --no-color          Disable colored output
    -h, --help          Show this help message
    --version           Show version information

Examples:
    ${SCRIPT_NAME} deploy.sh              # Preview changes
    ${SCRIPT_NAME} --apply deploy.sh      # Apply changes
    ${SCRIPT_NAME} --dir ./scripts        # Scan directory
    ${SCRIPT_NAME} --apply --dir ./ci     # Apply to all files in directory

Supported file types:
    *.sh, *.bash, *.yml, *.yaml, Makefile, Jenkinsfile, Dockerfile

EOF
}

show_version() {
    echo "${SCRIPT_NAME} version ${VERSION}"
    echo "Part of Keystone Core CLI migration tools"
}

# Migration patterns (sed-compatible)
# Format: "old_pattern|new_pattern|description"
MIGRATIONS=(
    # Policy -> Audit
    "kscorectl policy audit|kscorectl audit log|Policy audit moved to kscore-audit"
    "kscore-policy audit|kscore-audit log|Policy audit moved to kscore-audit"
    "kscorectl policy report|kscorectl audit report|Policy report moved to kscore-audit"
    "kscore-policy report|kscore-audit report|Policy report moved to kscore-audit"

    # GitOps -> Webhook
    "kscorectl gitops webhook list|kscorectl webhook list|GitOps webhook moved to kscore-webhook"
    "kscore-gitops webhook list|kscore-webhook list|GitOps webhook moved to kscore-webhook"
    "kscorectl gitops webhook test|kscorectl webhook test|GitOps webhook moved to kscore-webhook"
    "kscore-gitops webhook test|kscore-webhook test|GitOps webhook moved to kscore-webhook"

    # Cluster -> Cluster-backup
    "kscorectl cluster backup|kscorectl cluster-backup create|Cluster backup moved to kscore-cluster-backup"
    "kscore-cluster backup|kscore-cluster-backup create|Cluster backup moved to kscore-cluster-backup"
    "kscorectl cluster restore|kscorectl cluster-backup restore|Cluster restore moved to kscore-cluster-backup"
    "kscore-cluster restore|kscore-cluster-backup restore|Cluster restore moved to kscore-cluster-backup"

    # Files -> Files-storage
    "kscorectl files backend|kscorectl files-storage backend|Files backend moved to kscore-files-storage"
    "kscore-files backend|kscore-files-storage backend|Files backend moved to kscore-files-storage"
    "kscorectl files mirrors|kscorectl files-storage mirrors|Files mirrors moved to kscore-files-storage"
    "kscore-files mirrors|kscore-files-storage mirrors|Files mirrors moved to kscore-files-storage"

    # Identity -> Federation
    "kscorectl identity federation|kscorectl federation|Identity federation moved to kscore-federation"
    "kscore-identity federation|kscore-federation|Identity federation moved to kscore-federation"

    # Blueprint -> Blueprint-publish
    "kscorectl blueprint publish|kscorectl blueprint-publish publish|Blueprint publish moved to kscore-blueprint-publish"
    "kscore-blueprint publish|kscore-blueprint-publish publish|Blueprint publish moved to kscore-blueprint-publish"
    "kscorectl blueprint sign|kscorectl blueprint-publish sign|Blueprint sign moved to kscore-blueprint-publish"
    "kscore-blueprint sign|kscore-blueprint-publish sign|Blueprint sign moved to kscore-blueprint-publish"
    "kscorectl blueprint verify|kscorectl blueprint-publish verify|Blueprint verify moved to kscore-blueprint-publish"
    "kscore-blueprint verify|kscore-blueprint-publish verify|Blueprint verify moved to kscore-blueprint-publish"
    "kscorectl blueprint versions|kscorectl blueprint-publish versions|Blueprint versions moved to kscore-blueprint-publish"
    "kscore-blueprint versions|kscore-blueprint-publish versions|Blueprint versions moved to kscore-blueprint-publish"
    "kscorectl blueprint docs|kscorectl blueprint-publish docs|Blueprint docs moved to kscore-blueprint-publish"
    "kscore-blueprint docs|kscore-blueprint-publish docs|Blueprint docs moved to kscore-blueprint-publish"

    # Blueprint -> Blueprint-state
    "kscorectl blueprint rollback|kscorectl blueprint-state rollback|Blueprint rollback moved to kscore-blueprint-state"
    "kscore-blueprint rollback|kscore-blueprint-state rollback|Blueprint rollback moved to kscore-blueprint-state"
    "kscorectl blueprint snapshot|kscorectl blueprint-state snapshot|Blueprint snapshot moved to kscore-blueprint-state"
    "kscore-blueprint snapshot|kscore-blueprint-state snapshot|Blueprint snapshot moved to kscore-blueprint-state"
)

# File patterns to scan
FILE_PATTERNS="*.sh *.bash *.yml *.yaml Makefile Jenkinsfile Dockerfile *.groovy *.pipeline"

# Parse arguments
APPLY=false
SCAN_DIR=false
DRY_RUN=true
QUIET=false
VERBOSE=false
NO_BACKUP=false
TARGET=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -a|--apply)
            APPLY=true
            DRY_RUN=false
            shift
            ;;
        -d|--dir)
            SCAN_DIR=true
            shift
            ;;
        -n|--dry-run)
            DRY_RUN=true
            APPLY=false
            shift
            ;;
        -q|--quiet)
            QUIET=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        --no-backup)
            NO_BACKUP=true
            shift
            ;;
        --no-color)
            RED=''
            GREEN=''
            YELLOW=''
            BLUE=''
            NC=''
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        --version)
            show_version
            exit 0
            ;;
        -*)
            echo "Error: Unknown option: $1" >&2
            usage
            exit 1
            ;;
        *)
            TARGET="$1"
            shift
            ;;
    esac
done

if [[ -z "$TARGET" ]]; then
    echo "Error: No target file or directory specified" >&2
    usage
    exit 1
fi

log_info() {
    if [[ "$QUIET" != "true" ]]; then
        echo -e "${BLUE}INFO:${NC} $1"
    fi
}

log_success() {
    echo -e "${GREEN}SUCCESS:${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}WARNING:${NC} $1"
}

log_error() {
    echo -e "${RED}ERROR:${NC} $1" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}VERBOSE:${NC} $1"
    fi
}

# Check if file contains any deprecated commands
check_file() {
    local file="$1"
    local found=false

    for migration in "${MIGRATIONS[@]}"; do
        local old_pattern="${migration%%|*}"
        if grep -q "$old_pattern" "$file" 2>/dev/null; then
            found=true
            break
        fi
    done

    echo "$found"
}

# Show diff for a file
show_diff() {
    local file="$1"
    local temp_file
    temp_file=$(mktemp)

    cp "$file" "$temp_file"

    for migration in "${MIGRATIONS[@]}"; do
        IFS='|' read -r old_pattern new_pattern description <<< "$migration"
        # Use perl for more reliable replacement
        perl -i -pe "s/\Q${old_pattern}\E/${new_pattern}/g" "$temp_file"
    done

    if ! diff -q "$file" "$temp_file" > /dev/null 2>&1; then
        echo -e "\n${YELLOW}Changes in ${file}:${NC}"
        diff -u --color=auto "$file" "$temp_file" 2>/dev/null || diff -u "$file" "$temp_file"
        rm -f "$temp_file"
        return 0
    fi

    rm -f "$temp_file"
    return 1
}

# Apply migrations to a file
apply_migrations() {
    local file="$1"
    local changes_made=false

    # Create backup unless disabled
    if [[ "$NO_BACKUP" != "true" ]]; then
        cp "$file" "${file}.bak"
        log_verbose "Created backup: ${file}.bak"
    fi

    for migration in "${MIGRATIONS[@]}"; do
        IFS='|' read -r old_pattern new_pattern description <<< "$migration"

        if grep -q "$old_pattern" "$file" 2>/dev/null; then
            # Use perl for reliable in-place replacement
            perl -i -pe "s/\Q${old_pattern}\E/${new_pattern}/g" "$file"
            log_info "  Migrated: ${old_pattern} -> ${new_pattern}"
            changes_made=true
        fi
    done

    if [[ "$changes_made" == "true" ]]; then
        log_success "Applied migrations to: $file"
    fi
}

# Process a single file
process_file() {
    local file="$1"

    if [[ ! -f "$file" ]]; then
        log_error "File not found: $file"
        return 1
    fi

    if [[ ! -r "$file" ]]; then
        log_error "Cannot read file: $file"
        return 1
    fi

    local has_deprecated
    has_deprecated=$(check_file "$file")

    if [[ "$has_deprecated" == "true" ]]; then
        if [[ "$DRY_RUN" == "true" ]]; then
            show_diff "$file"
        else
            apply_migrations "$file"
        fi
    else
        log_verbose "No deprecated commands found in: $file"
    fi
}

# Find and process files in a directory
process_directory() {
    local dir="$1"
    local files_found=0
    local files_with_changes=0

    if [[ ! -d "$dir" ]]; then
        log_error "Directory not found: $dir"
        return 1
    fi

    log_info "Scanning directory: $dir"

    # Build find command
    local find_args=()
    for pattern in $FILE_PATTERNS; do
        find_args+=(-name "$pattern" -o)
    done
    # Remove last -o
    unset 'find_args[-1]'

    while IFS= read -r -d '' file; do
        ((files_found++))

        local has_deprecated
        has_deprecated=$(check_file "$file")

        if [[ "$has_deprecated" == "true" ]]; then
            ((files_with_changes++))
            if [[ "$DRY_RUN" == "true" ]]; then
                show_diff "$file"
            else
                apply_migrations "$file"
            fi
        fi
    done < <(find "$dir" -type f \( "${find_args[@]}" \) -print0 2>/dev/null)

    echo ""
    log_info "Scanned $files_found files, found $files_with_changes with deprecated commands"
}

# Summary of all migrations
show_migration_summary() {
    echo ""
    echo -e "${BLUE}Epic 30 Command Migration Summary${NC}"
    echo "================================="
    echo ""
    echo "The following commands have been restructured:"
    echo ""
    echo -e "${YELLOW}Policy -> Audit:${NC}"
    echo "  kscorectl policy audit  ->  kscorectl audit log"
    echo "  kscorectl policy report ->  kscorectl audit report"
    echo ""
    echo -e "${YELLOW}GitOps -> Webhook:${NC}"
    echo "  kscorectl gitops webhook list  ->  kscorectl webhook list"
    echo "  kscorectl gitops webhook test  ->  kscorectl webhook test"
    echo ""
    echo -e "${YELLOW}Cluster -> Cluster-backup:${NC}"
    echo "  kscorectl cluster backup   ->  kscorectl cluster-backup create"
    echo "  kscorectl cluster restore  ->  kscorectl cluster-backup restore"
    echo ""
    echo -e "${YELLOW}Files -> Files-storage:${NC}"
    echo "  kscorectl files backend  ->  kscorectl files-storage backend"
    echo "  kscorectl files mirrors  ->  kscorectl files-storage mirrors"
    echo ""
    echo -e "${YELLOW}Identity -> Federation:${NC}"
    echo "  kscorectl identity federation  ->  kscorectl federation"
    echo ""
    echo -e "${YELLOW}Blueprint -> Blueprint-publish:${NC}"
    echo "  kscorectl blueprint publish  ->  kscorectl blueprint-publish publish"
    echo "  kscorectl blueprint sign     ->  kscorectl blueprint-publish sign"
    echo "  kscorectl blueprint verify   ->  kscorectl blueprint-publish verify"
    echo "  kscorectl blueprint docs     ->  kscorectl blueprint-publish docs"
    echo ""
    echo -e "${YELLOW}Blueprint -> Blueprint-state:${NC}"
    echo "  kscorectl blueprint rollback  ->  kscorectl blueprint-state rollback"
    echo "  kscorectl blueprint snapshot  ->  kscorectl blueprint-state snapshot"
    echo ""
    echo "For more details, see: https://docs.keystonecore.io/cli/migration/epic-30"
}

# Main execution
main() {
    if [[ "$VERBOSE" == "true" ]]; then
        show_migration_summary
    fi

    if [[ "$SCAN_DIR" == "true" ]]; then
        process_directory "$TARGET"
    else
        process_file "$TARGET"
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        echo ""
        log_info "This was a dry run. Use --apply to make changes."
    fi
}

main
