# Ash Busybox Test Fix Plan

## Status
- many_ifs.tests now passes and completes in ~19s.
- Full ash test suite has not been re-run since latest fixes.

## Recent Fixes
- Implemented internal `expr` builtin and fast command-substitution path for simple `expr`.
- Fixed case matching for quoted patterns and patterns with leading spaces.
- Corrected IFS word splitting (non-whitespace delimiters, trailing empty fields).
- Reworked read field splitting to match BusyBox edge cases.
- Added sequential builtin-only pipeline execution for performance.
- Shared parse caches across command substitutions to reduce overhead.
- Run background builtins/functions internally (no exec) with subshell job tracking.

## Next Steps
1. Re-run full ash busybox diff suite and update pass/fail counts.
2. Triage remaining failures by category (function defs, command builtin, errors, local, subshells).
3. Remove any leftover debug-only code if found.
