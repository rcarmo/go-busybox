# ash fix log

Tracking the 21 fixes applied to reach 349/349 on the BusyBox ash test suite.

## Result

**349/349 (100%)**. Started at 328/349 (21 failures).

| Category | Pass | Total |
|---|---|---|
| ash-alias | 5 | 5 |
| ash-arith | 6 | 6 |
| ash-comm | 3 | 3 |
| ash-getopts | 8 | 8 |
| ash-glob | 10 | 10 |
| ash-heredoc | 25 | 25 |
| ash-invert | 3 | 3 |
| ash-misc | 99 | 99 |
| ash-parsing | 35 | 35 |
| ash-quoting | 24 | 24 |
| ash-read | 10 | 10 |
| ash-redir | 27 | 27 |
| ash-signals | 22 | 22 |
| ash-standalone | 6 | 6 |
| ash-vars | 69 | 69 |
| ash-z_slow | 3 | 3 |
| **Total** | **349** | **349** |

## Fixes

### Parser/syntax (7)
1. **Backslash-newline continuation** — joins at line-read level; `lineContMarker` for offsets.
2. **`if` across `\<newline>`** — `peekNextNonMarker` for operator lookahead.
3. **`$empty""` and `$empty''` empty word** — `containsUnescapedQuote` check.
4. **`"$@"` with no args → zero words** — special case in word expansion and for-loop.
5. **`if()` reserved-word syntax error** — detect reserved words as function names.
6. **Case pattern `()` quoting** — `findCasePatternClose` tracks single/double quotes.
7. **Alias in case body** — disable alias expansion when collecting case compound.

### Variable/expansion (7)
8. **Prefix `IFS=` for all builtins** — `IFS="," read ...` now works.
9. **`${x#'*'}` pattern quoting in heredoc** — removed premature `stripOuterQuotes`.
10. **Single-quote pattern in `${x#'\\\\'}`** — pattern quoting fix.
11. **Backtick inside single quotes** — track backtick regions when scanning.
12. **Command-sub newline stripping** — `TrimRight` instead of `TrimSuffix` for trailing `\n`.
13. **`unset -ff` flag** — accept repeated `-f`/`-v` flags.
14. **`\<newline>` at EOF** — only strip if original script had trailing newline.

### Redirections (3)
15. **`exec 1>&-` persistence** — update `savedStdio` before defer return.
16. **`1>&-`/`2>&-` parsing** — handle fd close before checking next token.
17. **`exec <file` + `read` + `cat`** — persist `bufio.Reader` wrapping in `savedStdio.In`.

### Signals (4)
18. **Trap exit codes** — `kill` in background runs via `startSubshellBackground` with `forwardSignal`.
19. **Nested signal traps** — inherited signal propagation from subshell to parent.
20. **`return` in trap handler** — `kill -s USR1 $$` inside subshell propagates via `forwardSignal`.
21. **Subshell signal reset** — real OS signals terminate subshell (not propagated).

### Performance
- **Pipeline fast path** — `echo`/`printf | (subshell)` avoids goroutine+runner copy overhead.
- **`readReady` for in-memory readers** — detect `Len()` on `bytes.Reader` etc.
