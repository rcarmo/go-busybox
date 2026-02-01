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
- [ ] Set up TinyGo WASM build pipeline
- [ ] Implement core runtime with basic syscall support
- [ ] Port initial utilities: `echo`, `cat`, `ls`, `cp`, `mv`, `rm`
- [ ] Create basic test harness

### Phase 2: Core Utilities
- [ ] File utilities: `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `grep`
- [ ] Directory utilities: `mkdir`, `rmdir`, `pwd`, `find`
- [ ] Text utilities: `sed`, `awk`, `tr`, `diff`
- [ ] Implement filesystem sandbox

### Phase 3: Advanced Features
- [ ] Shell implementation (`ash` subset)
- [ ] Process utilities: `ps`, `kill`, `xargs`
- [ ] Archive utilities: `tar`, `gzip`, `gunzip`
- [ ] Network utilities (sandboxed and gated via environment variable/CLI options): `wget`, `nc`

### Phase 4: Hardening
- [ ] Comprehensive fuzzing campaign
- [ ] Security audit
- [ ] Performance optimization
- [ ] Documentation and examples

## Testing Strategy

### Unit Testing
- Per-applet unit tests for core logic
- Mock filesystem and I/O for isolation

### Integration Testing
- End-to-end tests comparing output with C busybox
- Test across multiple WASM runtimes (TBD)

### Comparative Testing
- Automated test generation from busybox test suite
- Output diffing with normalization for acceptable variations
- Exit code and stderr validation

### Fuzz Testing
- **Input fuzzing**: Random/mutated command-line arguments and stdin
- **Corpus seeding**: Real-world usage patterns and edge cases
- **Differential fuzzing**: Compare WASM output vs C binary for same inputs
- Coverage-guided fuzzing using libFuzzer or go-fuzz
- Continuous fuzzing via OSS-Fuzz integration

### Performance Testing
- Benchmark suite for critical paths
- Memory profiling
- Startup latency measurements
