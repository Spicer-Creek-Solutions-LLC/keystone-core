#!/usr/bin/env bash
set -euo pipefail

FILE="${1:-AGENTS.md}"
MAX_WORDS="${MAX_WORDS:-700}"
WARN_WORDS="${WARN_WORDS:-600}"

if [[ ! -f "$FILE" ]]; then
  echo "ERROR: File not found: $FILE"
  exit 1
fi

words="$(wc -w < "$FILE" | tr -d ' ')"
echo "AGENTS.md word count: $words (warn>$WARN_WORDS, fail>$MAX_WORDS)"

if (( words > MAX_WORDS )); then
  echo "ERROR: $FILE exceeds max word budget ($MAX_WORDS)"
  exit 1
fi

if (( words > WARN_WORDS )); then
  echo "WARNING: $FILE exceeded warning threshold ($WARN_WORDS)"
fi

required_phrases=(
  'Non-Negotiable Workflow: `TODO.md`'
  'Commit Attribution Requirements'
  '### Required tests'
  '### Required docs'
  '## 7) Source-of-Truth Index'
)

for phrase in "${required_phrases[@]}"; do
  if ! grep -Fq "$phrase" "$FILE"; then
    echo "ERROR: Missing required section/phrase: $phrase"
    exit 1
  fi
done

# Volatile status content that should not be hardcoded in AGENTS.md.
deny_patterns=(
  '## Epic Status'
  '### Recently Completed'
  '### Future \(Unnumbered\)'
  '\| Epic \|'
  '\*\*Current Status\*\*: Epics'
  '\*\*Total\*\*: [0-9]+ binaries'
  '\bEpics [0-9]+'
  '[0-9]+ lines removed'
  'Total Test Functions'
)

for pattern in "${deny_patterns[@]}"; do
  if grep -Enq "$pattern" "$FILE"; then
    echo "ERROR: Volatile status data detected (pattern: $pattern)"
    grep -En "$pattern" "$FILE" || true
    exit 1
  fi
done

echo "AGENTS.md checks passed."
