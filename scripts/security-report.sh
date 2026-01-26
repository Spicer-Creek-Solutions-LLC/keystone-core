#!/bin/bash
# security-report.sh - Generate a markdown security report from all security scans
#
# This script runs all security checks and produces a unified markdown report
# with findings from each tool.

set -o pipefail

# Configuration
REPORT_DIR="${REPORT_DIR:-build/security}"
REPORT_FILE="${REPORT_FILE:-$REPORT_DIR/security-report.md}"
SECURITY_CACHE_DIR="${SECURITY_CACHE_DIR:-$PWD/.cache/security}"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)}"

# Container images
SECURITY_GO_IMAGE="${SECURITY_GO_IMAGE:-golang:1.25}"
SECURITY_GITLEAKS_IMAGE="${SECURITY_GITLEAKS_IMAGE:-zricethezav/gitleaks:latest}"
SECURITY_TRIVY_IMAGE="${SECURITY_TRIVY_IMAGE:-aquasec/trivy:latest}"
SECURITY_GOSEC_IMAGE="${SECURITY_GOSEC_IMAGE:-securego/gosec:latest}"
SECURITY_SEMGREP_IMAGE="${SECURITY_SEMGREP_IMAGE:-semgrep/semgrep:latest}"
SECURITY_GRYPE_IMAGE="${SECURITY_GRYPE_IMAGE:-anchore/grype:latest}"
SECURITY_KICS_IMAGE="${SECURITY_KICS_IMAGE:-checkmarx/kics:latest}"
SECURITY_HADOLINT_IMAGE="${SECURITY_HADOLINT_IMAGE:-hadolint/hadolint:latest}"
SECURITY_SCORECARD_IMAGE="${SECURITY_SCORECARD_IMAGE:-gcr.io/openssf/scorecard:stable}"

# Container run configuration
WORKDIR="/workspace"
CONTAINER_RUN="$CONTAINER_ENGINE run --rm -v $PWD:$WORKDIR \
    -v $SECURITY_CACHE_DIR/go:/tmp/go \
    -v $SECURITY_CACHE_DIR/gomod:/tmp/gomod \
    -v $SECURITY_CACHE_DIR/gocache:/tmp/gocache \
    -v $SECURITY_CACHE_DIR/trivy:/tmp/trivy \
    -v $SECURITY_CACHE_DIR/semgrep:/tmp/semgrep \
    -v $SECURITY_CACHE_DIR/grype:/tmp/grype \
    -w $WORKDIR"
GO_ENV="-e GOPATH=/tmp/go -e GOMODCACHE=/tmp/gomod -e GOCACHE=/tmp/gocache \
    -e PATH=/usr/local/go/bin:/tmp/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

# Exit codes for each scan
GITLEAKS_EXIT=0
GOVULNCHECK_EXIT=0
NANCY_EXIT=0
TRIVY_EXIT=0
GOSEC_EXIT=0
SEMGREP_EXIT=0
LICENSES_EXIT=0
GRYPE_EXIT=0
KICS_EXIT=0
HADOLINT_EXIT=0
SCORECARD_EXIT=0

# Ensure directories exist
mkdir -p "$REPORT_DIR"
mkdir -p "$SECURITY_CACHE_DIR"/{go,gomod,gocache,trivy,semgrep,grype}

# Helper function to run a scan and capture output
run_scan() {
    local name="$1"
    local output_file="$REPORT_DIR/${name}.txt"
    shift
    echo "Running $name..." >&2
    "$@" > "$output_file" 2>&1
    local exit_code=$?
    echo "$exit_code"
    return $exit_code
}

# Generate report header
cat > "$REPORT_FILE" << 'EOF'
# Security Scan Report

EOF
echo "**Generated:** $(date -u '+%Y-%m-%d %H:%M:%S UTC')" >> "$REPORT_FILE"
echo "**Repository:** $(basename "$PWD")" >> "$REPORT_FILE"
echo "**Commit:** $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')" >> "$REPORT_FILE"
echo "**Branch:** $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Run all scans and collect results

echo "=== Running security scans ===" >&2

# 1. Gitleaks (secrets detection)
echo "--- Gitleaks ---" >&2
$CONTAINER_RUN $SECURITY_GITLEAKS_IMAGE \
    detect --source $WORKDIR --config $WORKDIR/.gitleaks.toml --verbose \
    > "$REPORT_DIR/gitleaks.txt" 2>&1
GITLEAKS_EXIT=$?

# 2. Govulncheck (Go vulnerability database)
echo "--- Govulncheck ---" >&2
$CONTAINER_RUN $GO_ENV $SECURITY_GO_IMAGE sh -c '
    go install golang.org/x/vuln/cmd/govulncheck@latest 2>/dev/null
    govulncheck ./... 2>&1
' > "$REPORT_DIR/govulncheck.txt" 2>&1
GOVULNCHECK_EXIT=$?

# 3. Nancy (Sonatype OSS Index)
echo "--- Nancy ---" >&2
$CONTAINER_RUN $GO_ENV $SECURITY_GO_IMAGE sh -c '
    go install github.com/sonatype-nexus-community/nancy@latest 2>/dev/null
    go list -json -deps ./... 2>/dev/null | nancy sleuth 2>&1
' > "$REPORT_DIR/nancy.txt" 2>&1
NANCY_EXIT=$?

# 4. Trivy (filesystem scan)
echo "--- Trivy ---" >&2
$CONTAINER_RUN -e TRIVY_CACHE_DIR=/tmp/trivy $SECURITY_TRIVY_IMAGE \
    fs --severity HIGH,CRITICAL --format table $WORKDIR \
    > "$REPORT_DIR/trivy.txt" 2>&1
TRIVY_EXIT=$?

# 5. Gosec (Go security static analysis)
echo "--- Gosec ---" >&2
$CONTAINER_RUN --entrypoint /bin/gosec $SECURITY_GOSEC_IMAGE \
    -severity high -exclude=G115,G404,G101 -exclude-dir=test -exclude-dir=modules -exclude-dir=.cache ./... \
    > "$REPORT_DIR/gosec.txt" 2>&1
GOSEC_EXIT=$?

# 6. Semgrep (SAST)
echo "--- Semgrep ---" >&2
$CONTAINER_ENGINE run --rm -v "$PWD:/src" -w /src \
    -e SEMGREP_CACHE_DIR=/tmp/semgrep \
    --entrypoint semgrep $SECURITY_SEMGREP_IMAGE \
    scan --config auto --config p/golang --exclude .cache --json \
    > "$REPORT_DIR/semgrep.json" 2>&1
SEMGREP_EXIT=$?

# 7. License check
echo "--- Licenses ---" >&2
$CONTAINER_RUN $GO_ENV $SECURITY_GO_IMAGE sh -c '
    go install github.com/google/go-licenses@latest 2>/dev/null
    go-licenses check --ignore modernc.org/mathutil ./... 2>&1
' > "$REPORT_DIR/licenses.txt" 2>&1
LICENSES_EXIT=$?

# 8. Grype (vulnerability scanner with SBOM support)
echo "--- Grype ---" >&2
$CONTAINER_ENGINE run --rm -v "$PWD:$WORKDIR" -w $WORKDIR \
    -v "$SECURITY_CACHE_DIR/grype:/tmp/grype" \
    -e GRYPE_DB_CACHE_DIR=/tmp/grype \
    $SECURITY_GRYPE_IMAGE \
    dir:. --only-fixed --fail-on high \
    --exclude './.cache/**' --exclude './docs/node_modules/**' --exclude './modules/sdk/**' \
    > "$REPORT_DIR/grype.txt" 2>&1
GRYPE_EXIT=$?

# 9. KICS (Infrastructure as Code security)
echo "--- KICS ---" >&2
$CONTAINER_ENGINE run --rm -v "$PWD:$WORKDIR" -w $WORKDIR \
    $SECURITY_KICS_IMAGE scan \
    --path $WORKDIR \
    --exclude-paths ".cache,docs/node_modules,docs/themes,modules/sdk,examples,deploy/grafana,test/bootstrap/vm" \
    --exclude-queries "a88baa34-e2ad-44ea-ad6f-8cac87bc7c71" \
    --type Dockerfile,Kubernetes,DockerCompose \
    --fail-on high \
    --output-path /tmp \
    > "$REPORT_DIR/kics.txt" 2>&1
KICS_EXIT=$?

# 10. Hadolint (Dockerfile linting)
echo "--- Hadolint ---" >&2
# Find all Dockerfiles and lint them (excluding vendor/submodule directories)
DOCKERFILES=$(find . -name "Dockerfile*" -not -path "./.cache/*" -not -path "./docs/node_modules/*" -not -path "./docs/themes/*" -not -path "./modules/sdk/*" 2>/dev/null)
if [ -n "$DOCKERFILES" ]; then
    echo "Scanning Dockerfiles:" > "$REPORT_DIR/hadolint.txt"
    HADOLINT_EXIT=0
    for df in $DOCKERFILES; do
        echo "" >> "$REPORT_DIR/hadolint.txt"
        echo "=== $df ===" >> "$REPORT_DIR/hadolint.txt"
        $CONTAINER_ENGINE run --rm -i $SECURITY_HADOLINT_IMAGE < "$df" >> "$REPORT_DIR/hadolint.txt" 2>&1
        [ $? -ne 0 ] && HADOLINT_EXIT=1
    done
else
    echo "No Dockerfiles found." > "$REPORT_DIR/hadolint.txt"
    HADOLINT_EXIT=0
fi

# 11. Scorecard (OpenSSF security health metrics)
echo "--- Scorecard ---" >&2
# Scorecard requires a GitHub repo URL - check if we have one
REPO_URL=$(git config --get remote.origin.url 2>/dev/null || echo "")
if [[ "$REPO_URL" == *"github.com"* ]]; then
    # Convert SSH URL to HTTPS if needed
    REPO_URL=$(echo "$REPO_URL" | sed -e 's|git@github.com:|https://github.com/|' -e 's|\.git$||')
    $CONTAINER_ENGINE run --rm \
        -e GITHUB_AUTH_TOKEN="${GITHUB_TOKEN:-}" \
        $SECURITY_SCORECARD_IMAGE \
        --repo="$REPO_URL" \
        --format=json \
        > "$REPORT_DIR/scorecard.json" 2>&1
    SCORECARD_EXIT=$?
else
    echo "Scorecard requires a GitHub repository. Skipping." > "$REPORT_DIR/scorecard.json"
    SCORECARD_EXIT=0  # Not a failure, just not applicable
fi

# Generate summary table
echo "## Summary" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "| Scanner | Status | Details |" >> "$REPORT_FILE"
echo "|---------|--------|---------|" >> "$REPORT_FILE"

status_icon() {
    if [ "$1" -eq 0 ]; then
        echo "✅ Pass"
    else
        echo "❌ Fail"
    fi
}

echo "| Gitleaks (secrets) | $(status_icon $GITLEAKS_EXIT) | Secret detection |" >> "$REPORT_FILE"
echo "| Govulncheck | $(status_icon $GOVULNCHECK_EXIT) | Go vulnerability database |" >> "$REPORT_FILE"
echo "| Nancy | $(status_icon $NANCY_EXIT) | Sonatype OSS Index |" >> "$REPORT_FILE"
echo "| Trivy | $(status_icon $TRIVY_EXIT) | Filesystem vulnerability scan |" >> "$REPORT_FILE"
echo "| Grype | $(status_icon $GRYPE_EXIT) | Anchore vulnerability scanner |" >> "$REPORT_FILE"
echo "| Gosec | $(status_icon $GOSEC_EXIT) | Go security static analysis |" >> "$REPORT_FILE"
echo "| Semgrep | $(status_icon $SEMGREP_EXIT) | SAST rules |" >> "$REPORT_FILE"
echo "| KICS | $(status_icon $KICS_EXIT) | Infrastructure as Code security |" >> "$REPORT_FILE"
echo "| Hadolint | $(status_icon $HADOLINT_EXIT) | Dockerfile best practices |" >> "$REPORT_FILE"
echo "| Scorecard | $(status_icon $SCORECARD_EXIT) | OpenSSF security metrics |" >> "$REPORT_FILE"
echo "| Licenses | $(status_icon $LICENSES_EXIT) | Dependency license check |" >> "$REPORT_FILE"

echo "" >> "$REPORT_FILE"

# Calculate overall status
TOTAL_FAILURES=0
[ $GITLEAKS_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $GOVULNCHECK_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
# Nancy often fails without API token, treat as warning
# [ $NANCY_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $TRIVY_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $GRYPE_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $GOSEC_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $SEMGREP_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $KICS_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $HADOLINT_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
# Scorecard is informational, don't count as failure
# [ $SCORECARD_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))
[ $LICENSES_EXIT -ne 0 ] && TOTAL_FAILURES=$((TOTAL_FAILURES + 1))

if [ $TOTAL_FAILURES -eq 0 ]; then
    echo "**Overall Status:** ✅ All checks passed" >> "$REPORT_FILE"
else
    echo "**Overall Status:** ⚠️ $TOTAL_FAILURES check(s) reported issues" >> "$REPORT_FILE"
fi

echo "" >> "$REPORT_FILE"
echo "---" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Detailed results for each scanner

# Gitleaks
echo "## Gitleaks (Secret Detection)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $GITLEAKS_EXIT -eq 0 ]; then
    echo "No secrets detected." >> "$REPORT_FILE"
else
    echo "**⚠️ Potential secrets found:**" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
    # Filter to show only findings, not debug output
    grep -E "(Secret|File|Line|Commit|Finding|RuleID)" "$REPORT_DIR/gitleaks.txt" 2>/dev/null | head -100 >> "$REPORT_FILE" || cat "$REPORT_DIR/gitleaks.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"

# Govulncheck
echo "## Govulncheck (Go Vulnerabilities)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if grep -q "No vulnerabilities found" "$REPORT_DIR/govulncheck.txt" 2>/dev/null; then
    echo "No vulnerabilities found in Go dependencies." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    # Show vulnerability summaries
    grep -E "^(Vulnerability|GO-|Found|Package|stdlib|Calls|Description)" "$REPORT_DIR/govulncheck.txt" 2>/dev/null | head -100 >> "$REPORT_FILE" || cat "$REPORT_DIR/govulncheck.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"

    # Check for known unfixable vulns
    if grep -qE "GO-2025-3547|GO-2025-3521" "$REPORT_DIR/govulncheck.txt" 2>/dev/null; then
        echo "" >> "$REPORT_FILE"
        echo "> **Note:** GO-2025-3547 and GO-2025-3521 are known k8s.io/kubernetes vulnerabilities" >> "$REPORT_FILE"
        echo "> with no available fix. These are indirect dependencies via client-go and are documented" >> "$REPORT_FILE"
        echo "> in SECURITY.md." >> "$REPORT_FILE"
    fi
fi
echo "" >> "$REPORT_FILE"

# Nancy
echo "## Nancy (Sonatype OSS Index)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if grep -qi "no vulnerable" "$REPORT_DIR/nancy.txt" 2>/dev/null || grep -qi "no vulnerabilities" "$REPORT_DIR/nancy.txt" 2>/dev/null; then
    echo "No vulnerabilities found via Sonatype OSS Index." >> "$REPORT_FILE"
elif grep -qi "skipped" "$REPORT_DIR/nancy.txt" 2>/dev/null || grep -qi "rate limit" "$REPORT_DIR/nancy.txt" 2>/dev/null; then
    echo "> **Note:** Nancy requires a Sonatype OSS Index API token for full results." >> "$REPORT_FILE"
    echo "> Set the \`OSSI_TOKEN\` environment variable to enable full scanning." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    head -100 "$REPORT_DIR/nancy.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"

# Trivy
echo "## Trivy (Filesystem Scan)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $TRIVY_EXIT -eq 0 ]; then
    echo "No HIGH or CRITICAL vulnerabilities found." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/trivy.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"

# Grype
echo "## Grype (Vulnerability Scanner)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $GRYPE_EXIT -eq 0 ]; then
    echo "No HIGH or CRITICAL vulnerabilities with available fixes found." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/grype.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"
echo "> **Note:** Grype scans with \`--only-fixed\` to show only vulnerabilities with available patches." >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Gosec
echo "## Gosec (Go Security Analysis)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $GOSEC_EXIT -eq 0 ]; then
    echo "No high-severity security issues found." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/gosec.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"
echo "> **Exclusions:** G115 (integer overflow false positives), G404 (weak random for non-crypto), G101 (false positive key/token variable names)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Semgrep
echo "## Semgrep (SAST)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $SEMGREP_EXIT -eq 0 ]; then
    echo "No security issues found by Semgrep rules." >> "$REPORT_FILE"
else
    # Parse JSON for findings count
    if command -v jq >/dev/null 2>&1 && [ -f "$REPORT_DIR/semgrep.json" ]; then
        FINDING_COUNT=$(jq '.results | length' "$REPORT_DIR/semgrep.json" 2>/dev/null || echo "unknown")
        echo "**Findings:** $FINDING_COUNT" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        echo '```json' >> "$REPORT_FILE"
        jq -r '.results[] | "[\(.extra.severity)] \(.path):\(.start.line) - \(.extra.message)"' "$REPORT_DIR/semgrep.json" 2>/dev/null | head -50 >> "$REPORT_FILE"
        echo '```' >> "$REPORT_FILE"
    else
        echo '```' >> "$REPORT_FILE"
        head -100 "$REPORT_DIR/semgrep.json" >> "$REPORT_FILE"
        echo '```' >> "$REPORT_FILE"
    fi
fi
echo "" >> "$REPORT_FILE"

# KICS
echo "## KICS (Infrastructure as Code Security)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $KICS_EXIT -eq 0 ]; then
    echo "No HIGH or CRITICAL IaC security issues found." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/kics.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"
echo "> **Scanned:** Dockerfile, Kubernetes manifests, Docker Compose files" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Hadolint
echo "## Hadolint (Dockerfile Linting)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $HADOLINT_EXIT -eq 0 ]; then
    echo "All Dockerfiles follow best practices." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/hadolint.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"

# Scorecard
echo "## Scorecard (OpenSSF Security Metrics)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if grep -q "requires a GitHub repository" "$REPORT_DIR/scorecard.json" 2>/dev/null; then
    echo "> **Note:** Scorecard requires a GitHub repository URL. This scan was skipped." >> "$REPORT_FILE"
elif [ -f "$REPORT_DIR/scorecard.json" ] && command -v jq >/dev/null 2>&1; then
    # Extract overall score and individual check scores
    OVERALL_SCORE=$(jq -r '.score // "N/A"' "$REPORT_DIR/scorecard.json" 2>/dev/null)
    if [ "$OVERALL_SCORE" != "null" ] && [ "$OVERALL_SCORE" != "N/A" ]; then
        echo "**Overall Score:** $OVERALL_SCORE / 10" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        echo "| Check | Score | Reason |" >> "$REPORT_FILE"
        echo "|-------|-------|--------|" >> "$REPORT_FILE"
        jq -r '.checks[] | "| \(.name) | \(.score)/10 | \(.reason // "N/A") |"' "$REPORT_DIR/scorecard.json" 2>/dev/null >> "$REPORT_FILE"
    else
        echo '```' >> "$REPORT_FILE"
        cat "$REPORT_DIR/scorecard.json" >> "$REPORT_FILE"
        echo '```' >> "$REPORT_FILE"
    fi
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/scorecard.json" 2>/dev/null || echo "Scorecard results not available"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"
echo "> **Note:** Scorecard measures security health of open source projects. Higher scores indicate better security practices." >> "$REPORT_FILE"
echo "> Set \`GITHUB_TOKEN\` environment variable for more accurate results." >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# Licenses
echo "## License Check" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
if [ $LICENSES_EXIT -eq 0 ]; then
    echo "All dependency licenses are acceptable." >> "$REPORT_FILE"
else
    echo '```' >> "$REPORT_FILE"
    cat "$REPORT_DIR/licenses.txt" >> "$REPORT_FILE"
    echo '```' >> "$REPORT_FILE"
fi
echo "" >> "$REPORT_FILE"

# Footer
echo "---" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "## Tool Versions" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "| Tool | Image |" >> "$REPORT_FILE"
echo "|------|-------|" >> "$REPORT_FILE"
echo "| Go | \`$SECURITY_GO_IMAGE\` |" >> "$REPORT_FILE"
echo "| Gitleaks | \`$SECURITY_GITLEAKS_IMAGE\` |" >> "$REPORT_FILE"
echo "| Trivy | \`$SECURITY_TRIVY_IMAGE\` |" >> "$REPORT_FILE"
echo "| Grype | \`$SECURITY_GRYPE_IMAGE\` |" >> "$REPORT_FILE"
echo "| Gosec | \`$SECURITY_GOSEC_IMAGE\` |" >> "$REPORT_FILE"
echo "| Semgrep | \`$SECURITY_SEMGREP_IMAGE\` |" >> "$REPORT_FILE"
echo "| KICS | \`$SECURITY_KICS_IMAGE\` |" >> "$REPORT_FILE"
echo "| Hadolint | \`$SECURITY_HADOLINT_IMAGE\` |" >> "$REPORT_FILE"
echo "| Scorecard | \`$SECURITY_SCORECARD_IMAGE\` |" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "" >&2
echo "=== Security report generated ===" >&2
echo "Report: $REPORT_FILE" >&2
echo "Raw output files: $REPORT_DIR/" >&2

# Exit with failure if any critical scanner failed
if [ $TOTAL_FAILURES -gt 0 ]; then
    exit 1
fi
exit 0
