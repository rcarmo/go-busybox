# Ash Busybox Test Fix Plan

## Status
- many_ifs.tests now passes and completes in ~19s.
- ash-quoting suite passes after backslash/pattern fixes.
- ash-vars suite now passes (var_in_pipes/var_bash*/var_posix1/var_unbackslash1 fixes).
- Full ash test suite has not been re-run since latest fixes.

## Recent Fixes
- Implemented internal `expr` builtin and fast command-substitution path for simple `expr`.
- Fixed case matching for quoted patterns and patterns with leading spaces.
- Corrected IFS word splitting (non-whitespace delimiters, trailing empty fields).
- Reworked read field splitting to match BusyBox edge cases.
- Shared parse caches across command substitutions to reduce overhead.
- Fixed case pattern command substitutions by running them in the active runner.
- Corrected for-list terminator handling so keyword lists parse correctly.
- Reverted background command execution to OS processes (no internal background builtins).
- Fixed backslash handling in tokenization for single-quoted strings.
- Hardened case pattern parsing to ignore `)` inside bracket expressions.
- Improved glob/pattern handling for parameter expansion and case patterns (bracket escapes, alternation splitting, normalize glob escapes).
- Added paren-depth pipeline splitting and ensured prefix assignments apply in subshell/external envs.
- Fixed bash-style substring offsets, replacement expansion in ${var/pat/repl}, and literal backslash/glob escaping in patterns.
- Ensured function prefix assignments restore, `%*` keeps value for shortest suffix, and line-continuation `$\
(` parses as command substitution.

## Next Steps
1. Re-run full ash busybox diff suite and update pass/fail counts.
2. Triage any remaining failures by category (function defs, builtins, errors, local, subshells).
3. Remove any leftover debug-only code if found.
