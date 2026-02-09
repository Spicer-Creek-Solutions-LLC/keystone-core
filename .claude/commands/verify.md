---
description: Run the full validation pipeline (lint, docs-lint, tests) and report a summary
---

Run the full project validation pipeline in sequence. For each step, report whether it passed or failed. At the end, provide a summary table.

Steps to run:

1. **Lint** (`make lint`) - Run Go linters. Report the number of issues found.
2. **Docs Lint** (`make docs-lint-container`) - Run markdown linting in a container. Report the number of errors found.
3. **Tests** (`make test`) - Run the full test suite with race detection. Report the number of failures.

After all three steps complete, print a summary like:

| Check | Result |
|-------|--------|
| Lint | PASS (0 issues) |
| Docs Lint | PASS (0 errors) |
| Tests | PASS (0 failures) |

If any step fails, continue running the remaining steps so the user gets a complete picture. Do NOT attempt to fix any issues — just report them.

$ARGUMENTS
