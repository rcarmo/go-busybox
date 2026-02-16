# Ash Busybox Test Fix Plan

## Status: 234 PASS / 99 FAIL

## Priority Batches

### Batch 1 - Function definitions (~10 tests)
- `h ( ) { ... }` with space between parens
- `f() ( ... )` subshell body 
- `function f() { ... }` / `function f { ... }` bash form
- `f() for ...` compound body form
- Tests: func1, func4, func5, func_bash1, func_compound1

### Batch 2 - `command` builtin (~2 tests)
- command.tests, command2.tests

### Batch 3 - Error message formatting (~20+ tests)
- exec error format matching reference busybox
- Exit codes 126/127 for not found / permission denied
- `$THIS_SH -c` pattern

### Batch 4 - `local` builtin improvements (~4 tests)
- local1, local2, func_local1, func_local2
- Save/restore variable values properly

### Batch 5 - Subshell compound commands (~10 tests)
- `(while ...)`, `(case ...)`, etc. inside subshells

### Batch 6 - Signal handling in loops (~3 tests)
- exitcode_trap1, exitcode_trap3 (timeout)

### Batch 7 - Variable expansion (~50+ tests)
- Pattern replacement, substring, IFS, etc.

### Batch 8 - read builtin (~8 tests)
### Batch 9 - Redirection (~15 tests)
### Batch 10 - Quoting/escaping/glob (~20 tests)
### Batch 11 - Parsing (~15 tests)
### Batch 12 - Misc remaining
