---
description: Run tests, analyze failures, and fix them automatically
---

Run the test suite, identify all failures, and fix them. Follow this workflow:

1. **Run tests**: Execute `make test` and capture output.
2. **Identify failures**: Parse the output for FAIL lines and extract the failing packages and test names.
3. **If no failures**: Report that all tests pass and stop.
4. **Analyze each failure**: For each failing test:
   - Run the specific failing test with `-race -v -count=1` to get detailed output.
   - Read the relevant source and test files to understand the root cause.
   - Categorize the failure (data race, logic bug, flaky timing, missing dependency, etc.).
5. **Fix the failures**: Apply fixes to the source or test files as appropriate. Use parallel agents for independent fixes across different packages.
6. **Verify fixes**: Re-run the previously-failing tests with `-race -count=1` to confirm they pass.
7. **Full verification**: Run `make test` one final time to ensure no regressions.
8. **Report**: Provide a summary table of all failures found and how each was fixed.

Do NOT commit changes — leave that to the user.

$ARGUMENTS
