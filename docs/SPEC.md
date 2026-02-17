# Busybox WASM — Specification

Sandboxable busybox in Go, compiled to WASM via TinyGo. Extensive comparative testing (including fuzzing) against the reference C binary.

## Architecture

Three components:

1. **Applet library** — 57 utilities under `pkg/applets/`, each a self-contained package.
2. **Core runtime** — shared helpers (`core/`, `sandbox/`, `procutil/`), sandboxed filesystem via `pkg/core/fs/`.
3. **Test harness** — unit tests, fuzz tests, integration parity matrix, BusyBox reference suite.

```
go-busybox/
├── cmd/              # entry points (51 standalone + busybox multi-call)
├── pkg/
│   ├── applets/      # 57 applet packages + procutil
│   ├── core/         # fs/, archiveutil/, headtail, textutil
│   ├── integration/  # BusyBox comparison tests
│   ├── sandbox/      # capability-based access control
│   └── testutil/     # FuzzCompare, CaptureStdio, helpers
├── busybox-reference/# reference binary (v1.35.0) + test suite
└── docs/SPEC.md
```

### Sandboxing

- Capability-based filesystem: only pre-opened directories accessible
- Network gated: `wget`, `nc`, `dig`, `ss` require explicit opt-in
- Memory isolation via WASM linear memory
- Syscalls limited to WASI preview1

## Requirements

### Toolchain
- Go 1.22+, TinyGo 0.34+ (WASM target: `wasm32-wasi`)
- No CGO dependencies

### Compatibility
- BusyBox v1.35.0 as reference
- POSIX compliance where applicable

### Constraints
- Binary size: 4.7MB standard WASM, 2.0MB optimised (`-opt=z -no-debug`)
- Startup: <10ms cold start
- Memory: <16MB baseline
- 16 applets stubbed under WASI (OS-dependent syscalls); 41 fully functional

## Implementation phases

### Phase 1: Foundation ✅
- TinyGo WASM build pipeline
- Core runtime with syscall support
- Initial utilities: `echo`, `cat`, `ls`, `cp`, `mv`, `rm`
- Basic test harness

### Phase 2: Core utilities ✅
- File: `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `grep`
- Directory: `mkdir`, `rmdir`, `pwd`, `find`
- Text: `sed`, `tr`, `diff`, `awk` (via goawk)
- Filesystem sandbox

### Phase 3: Advanced ✅
- Shell: `ash` — near-complete POSIX shell, 349/349 BusyBox tests
- Process: `ps`, `kill`, `killall`, `pidof`, `pgrep`, `pkill`, `nice`, `renice`, `nohup`, `time`, `timeout`, `taskset`, `ionice`, `setsid`, `start-stop-daemon`, `xargs`, `top`, `watch`, `sleep`, `nproc`, `free`, `logname`, `uptime`, `users`, `who`, `w`, `whoami`
- Archive: `tar`, `gzip`, `gunzip`
- Network: `wget`, `nc`, `dig`, `ss`

### Phase 4: Hardening (current)
- [x] Comprehensive fuzzing (100 functions, 353 seeds)
- [x] Full Godoc coverage (all packages, all exported symbols)
- [x] BusyBox reference suite 387/387
- [ ] Security audit
- [ ] Performance optimisation

## Testing strategy

### Four layers

| Layer | Files | Scope |
|---|---|---|
| Unit tests | 62 `*_test.go` | per-applet logic |
| Fuzz tests | 58 `*_fuzz_test.go`, 100 functions, 353 seeds | crash/panic detection, differential comparison |
| Integration | `pkg/integration/` | parity matrix vs reference BusyBox |
| Reference suite | `busybox-reference/testsuite/` | 387/387 (308 new-style + 79 old-style) |

### Fuzz testing

Every applet has at least one fuzz test. Complex applets (sed, grep, awk, diff, tr, cut, tar, gzip, gunzip) have multiple functions covering:

- **Input fuzzing** — random data with fixed flags
- **Expression/DSL fuzzing** — fuzzed sed expressions, grep patterns, awk programmes, find predicates, tr character sets, cut field specs
- **Flag combination fuzzing** — fixed input with varied flag sets
- **Roundtrip fuzzing** — gzip→gunzip, tar create→extract, cp source→destination byte-for-byte verification
- **Invalid input fuzzing** — arbitrary bytes fed to gunzip, tar to verify graceful failure

Safety filters prevent non-termination: sed rejects branch-label loops and excessive regex quantifiers; printf skips non-consuming format strings; all inputs clamped to 2048 bytes.

### Parity coverage

All applets below pass their respective BusyBox reference tests at 100%:

| Applet | Tests | Source |
|---|---|---|
| ash | 349/349 | BusyBox ash test suite |
| awk | 53/53 | `awk.tests` |
| sed | 92/92 | `sed.tests` |
| grep | 44/44 | `grep.tests` |
| printf | 24/24 | `printf.tests` |
| cut | 22/22 | `cut.tests` |
| uniq | 14/14 | `uniq.tests` |
| cp | 13/13 | `cp.tests` + old-style |
| diff | 12/12 | `diff.tests` |
| xargs | 7/7 | `xargs.tests` |
| sort | 5/5 | `sort.tests` |
| gunzip | 5/5 | `gunzip.tests` |
| tar | 3/3 | `tar.tests` |
| taskset | 3/3 | `taskset.tests` |
| pidof | 3/3 | `pidof.tests` |
| find | 2/2 | `find.tests` |
| head | 2/2 | `head.tests` |
| tail | 2/2 | `tail.tests` + old-style |
| tr | 2/2 | `tr.tests` + old-style |
| wget | 4/4 | old-style |
| wc | 5/5 | old-style |
| cat, echo, gzip, ls, mkdir, mv, pwd, rm, rmdir | — | old-style (all pass) |

Remaining applets (`kill`, `killall`, `pgrep`, `pkill`, `nice`, `nohup`, `renice`, `ps`, `free`, `nproc`, `ionice`, `setsid`, `sleep`, `timeout`, `time`, `top`, `watch`, `logname`, `uptime`, `users`, `w`, `who`, `whoami`, `dig`, `nc`, `ss`, `start-stop-daemon`) have unit tests and fuzz tests but no dedicated BusyBox reference-suite entries.

### Parity normalisations

- Exit codes: BusyBox returns 1 for usage errors where we return 2 — both accepted.
- `ps` output: whitespace normalised.
- `find` output: leading `./` stripped.
- `pwd`: compared per temp directory.
- `taskset`: PID normalised.
- `wget` stderr: ignored.
- `time`/`uptime`/`who`/`w`: timing/session-dependent output excluded from strict comparison.
