# Busybox WASM

The goal of this project is to create a sandboxable version of busybox using WASM, by porting it entirely to `tinygo` and doing extensive comparative testing (including fuzzing) against the original C binary.

## Architecture

### Overview
The project consists of three main components:

1. **Core Runtime** - WASM runtime environment handling syscall emulation and sandboxing
2. **Applet Library** - Individual busybox utilities ported to TinyGo
3. **Test Harness** - Comparative testing framework against the original C busybox

### Components

```
busybox-wasm/
├── cmd/              # Entry points for each applet
├── pkg/
│   ├── applets/      # Individual utility implementations
│   ├── runtime/      # WASM runtime and syscall handling
│   └── sandbox/      # Sandboxing and capability management
├── testdata/         # Test fixtures and expected outputs
└── fuzz/             # Fuzzing corpus and harnesses
```

### Sandboxing Model
- Capability-based filesystem access
- Restricted network operations
- Memory isolation via WASM linear memory
- Configurable resource limits (CPU, memory, file descriptors)

## Technical Requirements

### Language & Toolchain
- **TinyGo** (latest stable) targeting `wasm32-wasi`
- Compatible with WASI preview1 interface
- No CGO dependencies

### Compatibility Targets
- Busybox v1.36.x as reference implementation
- POSIX compliance where applicable
- Support for common shell scripts using busybox utilities

### Constraints
- Binary size: Target <2MB for full build, <100KB per applet
- Startup time: <10ms cold start
- Memory usage: <16MB baseline

## Implementation Phases

### Phase 1: Foundation
- [x] Set up TinyGo WASM build pipeline
- [x] Implement core runtime with basic syscall support
- [x] Port initial utilities: `echo`, `cat`, `ls`, `cp`, `mv`, `rm`
- [x] Create basic test harness

### Phase 2: Core Utilities
- [x] File utilities: `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `grep`
- [x] Directory utilities: `mkdir`, `rmdir`, `pwd`, `find`
- [x] Text utilities: `sed`, `tr`, `diff`
- [x] `awk` parity via goawk (BusyBox testsuite)
- [x] Implement filesystem sandbox

### Phase 3: Advanced Features
- [x] Shell implementation (`ash` subset; BusyBox testsuite parity)
- [x] Process utilities: `ps`, `kill`, `xargs`, `killall`, `pidof`, `pgrep`, `pkill`, `nice`, `renice`, `uptime`, `who`, `w`, `top`, `time`, `nohup`, `watch`, `setsid`, `start-stop-daemon`, `sleep`, `timeout`, `taskset`, `ionice`, `nproc`, `free`, `logname`, `users`, `whoami` (baseline implementations complete; parity gaps tracked in TODOs/tests)
- [x] Archive utilities: `tar`, `gzip`, `gunzip` (baseline implemented)
- [x] Network utilities (sandboxed and gated via environment variable/CLI options): `wget`, `nc`, `dig`, `ss` (baseline implemented)

### Phase 4: Hardening
- [x] Comprehensive fuzzing campaign
- [ ] Security audit
- [ ] Performance optimization
- [x] Documentation and examples

## Testing Strategy

### Unit Testing
- Per-applet unit tests for core logic
- Mock filesystem and I/O for isolation

### Integration Testing
- End-to-end tests comparing output with C busybox
- Test across multiple WASM runtimes (TBD)

### Comparative Testing

### WASM Parity
- Integration tests run the BusyBox parity matrix against the unified busybox.wasm using wasmtime when available.
- Deviations are expected for network-facing applets (e.g. `dig`, `ss`) which are skipped in WASM parity runs.
- Automated test generation from busybox test suite
- Output diffing with normalization for acceptable variations
- Exit code and stderr validation

### Per-applet parity coverage
Coverage sources:
- BusyBox testsuite: `awk.tests`, `ash.tests`
- Integration parity matrix: `pkg/integration/busybox_compare_test.go`
- Others: baseline implementations without parity matrix/tests yet

| Applet | Parity coverage | Notes |
| --- | --- | --- |
| `ash` | BusyBox testsuite (`ash.tests`) | Line-edit focused tests. |
| `awk` | BusyBox testsuite (`awk.tests`) | goawk parity adaptations. |
| `cat` | Integration parity matrix |  |
| `cp` | Integration parity matrix |  |
| `cut` | Integration parity matrix |  |
| `diff` | Integration parity matrix |  |
| `dig` | None yet | Network-facing; parity skipped in WASM runs. |
| `echo` | Integration parity matrix |  |
| `find` | Integration parity matrix | BusyBox `./` path prefix is normalized. |
| `free` | Integration parity matrix |  |
| `grep` | Integration parity matrix |  |
| `gunzip` | Integration parity matrix |  |
| `gzip` | Integration parity matrix |  |
| `head` | Integration parity matrix |  |
| `ionice` | Integration parity matrix |  |
| `kill` | Integration parity matrix |  |
| `killall` | Integration parity matrix |  |
| `logname` | Integration parity matrix |  |
| `ls` | Integration parity matrix |  |
| `mkdir` | Integration parity matrix |  |
| `mv` | Integration parity matrix |  |
| `nc` | Integration parity matrix | Loopback-only parity checks. |
| `nice` | None yet | Baseline implementation only. |
| `nohup` | None yet | Baseline implementation only. |
| `nproc` | Integration parity matrix |  |
| `pgrep` | None yet | Baseline implementation only. |
| `pidof` | Integration parity matrix |  |
| `pkill` | None yet | Baseline implementation only. |
| `ps` | Integration parity matrix | Output normalization applied. |
| `pwd` | Integration parity matrix | Compared per temporary directory. |
| `renice` | Integration parity matrix |  |
| `rm` | Integration parity matrix |  |
| `rmdir` | Integration parity matrix |  |
| `sed` | Integration parity matrix |  |
| `setsid` | Integration parity matrix |  |
| `sleep` | Integration parity matrix |  |
| `sort` | Integration parity matrix |  |
| `ss` | None yet | Network-facing; parity skipped in WASM runs. |
| `start-stop-daemon` | Integration parity matrix | Skipped in CI parity run (output varies). |
| `tail` | Integration parity matrix |  |
| `tar` | Integration parity matrix |  |
| `taskset` | Integration parity matrix | PID output normalized. |
| `time` | Integration parity matrix | Output timing varies; excluded from strict compare. |
| `timeout` | Integration parity matrix |  |
| `top` | Integration parity matrix | Skipped in CI parity run (output varies). |
| `tr` | Integration parity matrix |  |
| `uniq` | Integration parity matrix |  |
| `uptime` | Integration parity matrix | Output varies across systems. |
| `users` | None yet | Baseline implementation only. |
| `w` | Integration parity matrix | Output varies across systems. |
| `watch` | None yet | No parity tests; output differs from BusyBox. |
| `wc` | Integration parity matrix |  |
| `wget` | Integration parity matrix | Loopback-only parity checks. |
| `who` | Integration parity matrix | Output varies across systems. |
| `whoami` | Integration parity matrix |  |
| `xargs` | Integration parity matrix |  |

Parity matrix normalizations: invalid option/missing file exit codes accept BusyBox=1 vs ours=2, `ps` output normalized, `find` strips leading `./`, `pwd` compares per temp directory, `taskset` PID normalized, `wget` stderr ignored, loopback-only `nc`/`wget` checks, and time/uptime/who/w skipped for strict output comparison.

### Fuzz Testing

### Fuzzing Harness
- Each applet includes a Go fuzz test (testing.F) with shared fixtures and BusyBox differential comparisons when available.
- Fuzzing clamps input sizes to keep runs deterministic and bounded.
- **Input fuzzing**: Random/mutated command-line arguments and stdin
- **Corpus seeding**: Real-world usage patterns and edge cases
- **Differential fuzzing**: Compare WASM output vs C binary for same inputs
- Coverage-guided fuzzing using libFuzzer or go-fuzz
- Continuous fuzzing via OSS-Fuzz integration

### Performance Testing
- Benchmark suite for critical paths
- Memory profiling
- Startup latency measurements
