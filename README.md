# go-busybox

<p align="center">
  <img src="docs/icon-256.png" alt="Busybox WASM" width="256" />
</p>

A WIP sandboxable implementation of busybox utilities in Go, intended for compiling to WebAssembly using TinyGo for use in sandboxed AI agents. It builds a unified `busybox` multi-call binary plus individual applet entry points under `cmd/` for native and WASM targets.

## Overview

This project ports common busybox utilities to Go, targeting WebAssembly (WASI) for secure, sandboxed execution. The current milestone prioritizes **OS process parity** (native exec/fork/wait semantics) to ensure correctness and test coverage before moving to a fully WASM-native execution model. It aims to provide:

- **Capability-based sandboxing** via WASM's memory isolation
- **POSIX-compatible utilities** for shell scripting
- **Comparative testing** against the original C busybox binary
- **Small binary sizes** (<100KB per applet, <2MB combined)
- **Envisioned dual use**: a WASM sandboxing tool and a way to extend [GoKrazy](https://gokrazy.org) on embedded devices

## Reference BusyBox

Current parity target: **BusyBox v1.35.0 (Debian 1:1.35.0-4+b7)** as installed on the test host.

### ash Test Suite Results

The Go `ash` implementation is validated against the reference C busybox using the full busybox ash test suite. Each `.tests` file is run under both shells and outputs are compared.

| Category | Pass | Total |
|----------|------|-------|
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
| **Total** | **349** | **349 (100%)** |

### Busybox Reference Test Suite Compatibility

The busybox reference test suite (`/workspace/busybox-reference/testsuite/`) is used as the golden standard. Results against all implemented applets:

| Applet | Pass | Total | Status |
|--------|------|-------|--------|
| awk | 53 | 53 | ✅ 100% |
| cp | 13 | 13 | ✅ 100% |
| cut | 22 | 22 | ✅ 100% |
| grep | 44 | 44 | ✅ 100% |
| printf | 24 | 24 | ✅ 100% |
| sort | 5 | 5 | ✅ 100% |
| tr | 2 | 2 | ✅ 100% |
| uniq | 14 | 14 | ✅ 100% |
| xargs | 7 | 7 | ✅ 100% |
| find | 2 | 2 | ✅ 100% |
| head | 2 | 2 | ✅ 100% |
| tail | 2 | 2 | ✅ 100% |
| diff | 11 | 12 | 91.7% |
| sed | 84 | 92 | 91.3% |
| pidof | 2 | 3 | 66.7% |
| taskset | 2 | 3 | 66.7% |
| **New-style total** | **289** | **308** | **93.8%** |

Old-style directory tests (cat, cp, cut, echo, ls, mkdir, mv, pwd, rm, rmdir, tail, tr, wc, wget): **75/79 (94.9%)**

**Combined: 364/387 (94.1%)**

## Feature Completeness Status

### Applet Implementation Status

| Category | Applet | Status | Notes |
|----------|--------|--------|-------|
| **Shell** | ash | 🟢 ~99% | Builtins complete; pipelines, redirects, control flow, functions, case/esac, arithmetic, command substitution, traps/signals — **349/349 busybox ash tests passing (100%)** |
| **Text Processing** | awk | 🟢 ~90% | Full parser/evaluator, builtins, printf/sprintf, getline, regex — **53/53 busybox tests (100%)** |
| | sed | 🟢 ~90% | BRE/ERE regex, in-place editing, hold space, branches/labels, backreferences — **84/92 busybox tests (91.3%)** |
| | grep | 🟢 Complete | -E/-F/-i/-v/-c/-l/-L/-n/-r/-w/-x/-o/-s/-e/-f flags — **44/44 busybox tests (100%)** |
| | cut | 🟢 Complete | Fields, characters, bytes, custom delimiters — **22/22 busybox tests (100%)** |
| | tr | 🟢 Complete | Translation, deletion, squeeze, POSIX classes — **2/2 busybox tests (100%)** |
| | sort | 🟢 Complete | Numeric, reverse, unique, key-based sorting — **5/5 busybox tests (100%)** |
| | uniq | 🟢 Complete | Count, duplicate, unique, skip fields/chars, max chars — **14/14 busybox tests (100%)** |
| | wc | 🟢 Complete | Lines, words, characters, bytes |
| | diff | 🟢 Complete | Unified diff, stdin support — **11/12 busybox tests (91.7%)** |
| | printf | 🟢 Complete | Full format spec, backreferences, %b escapes — **24/24 busybox tests (100%)** |
| **File Operations** | cat | 🟢 Complete | Number lines, show ends/tabs |
| | head | 🟢 Complete | Lines and bytes modes |
| | tail | 🟢 Complete | Lines, bytes, follow mode |
| | cp | 🟢 Complete | Recursive, preserve, symlink handling (-d/-P/-L/-H) — **13/13 busybox tests (100%)** |
| | mv | 🟢 Complete | Force, no-clobber, verbose |
| | rm | 🟢 Complete | Recursive, force, verbose |
| | ls | 🟢 Complete | Long format, hidden, recursive, sorting |
| | find | 🟢 Complete | Name, type, size, exec predicates |
| | mkdir | 🟢 Complete | Parents, mode |
| | rmdir | 🟢 Complete | Parents, ignore-fail |
| | pwd | 🟢 Complete | Physical/logical modes |
| **Archive** | tar | 🟢 Complete | Create, extract, gzip compression |
| | gzip | 🟢 Complete | Compression levels, keep, stdout |
| | gunzip | 🟢 Complete | Keep, stdout, force |
| **Process** | ps | 🟢 Complete | Process listing with various formats |
| | kill | 🟢 Complete | Signal sending by PID |
| | killall | 🟢 Complete | Signal by process name |
| | pgrep | 🟢 Complete | Pattern-based process search |
| | pkill | 🟢 Complete | Pattern-based signal sending |
| | pidof | 🟢 Complete | PID lookup by name |
| | nice | 🟢 Complete | Priority adjustment |
| | renice | 🟢 Complete | Priority modification |
| | nohup | 🟢 Complete | Ignore hangup signal |
| | timeout | 🟢 Complete | Command timeout with signals |
| | time | 🟢 Complete | Command timing |
| | xargs | 🟢 Complete | Build command lines from input |
| | start-stop-daemon | 🟡 Basic | Native-only; `--start`/`--exec` with optional `--pidfile` |
| **System** | uptime | 🟢 Complete | System uptime display |
| | free | 🟢 Complete | Memory usage |
| | nproc | 🟢 Complete | CPU count |
| | logname | 🟢 Complete | Login name |
| | whoami | 🟢 Complete | Current user |
| | who | 🟢 Complete | Logged-in users |
| | users | 🟢 Complete | User list |
| | w | 🟢 Complete | Who and what |
| **Network** | wget | 🟢 Complete | HTTP/HTTPS downloads |
| | nc | 🟢 Complete | Netcat TCP/UDP connections |
| | dig | 🟢 Complete | DNS lookup |
| | ss | 🟢 Complete | Socket statistics |
| **Other** | echo | 🟢 Complete | -n, -e flags |
| | sleep | 🟢 Complete | Seconds, subseconds |
| | watch | 🟢 Complete | Periodic command execution |
| | setsid | 🟢 Complete | New session leader |
| | ionice | 🟢 Complete | I/O scheduling class |
| | taskset | 🟢 Complete | CPU affinity |
| | top | 🟡 Basic | Process monitor (simplified) |

### Shell (ash) Feature Details

| Feature | Status | Notes |
|---------|--------|-------|
| **Parsing** | ✅ Complete | Tokenizer, quoting, escapes, backslash-newline continuation |
| **Pipelines** | ✅ Complete | Multi-stage with fast path optimization for simple builtins |
| **Redirections** | ✅ Complete | `<`, `>`, `>>`, `2>`, `2>>`, `>&`, `<&`, fd close (`>&-`) |
| **Control Flow** | ✅ Complete | if/elif/else/fi, while, until, for, case/esac |
| **Command Substitution** | ✅ Complete | `$(...)` and backticks, nested, with proper newline stripping |
| **Arithmetic** | ✅ Complete | `$((...))` with operators |
| **Functions** | ✅ Complete | Definition and positional params |
| **Parameter Expansion** | ✅ Complete | `${VAR:-default}`, `${#VAR}`, `${VAR##pattern}`, etc. |
| **Positional Params** | ✅ Complete | `$0`-`$9`, `$@`, `$*`, `$#`, shift |
| **Special Variables** | ✅ Complete | `$$`, `$?`, `$!`, `$PPID`, `$LINENO` |
| **File Tests** | ✅ Complete | -e, -f, -d, -r, -w, -x, -s, -L |
| **Builtins** | ✅ Complete | 25+ builtins including cd, export, eval, read, printf, alias, getopts, trap |
| **Background Jobs** | ✅ Complete | `&`, jobs/fg/wait with signal forwarding |
| **Here-documents** | ✅ Complete | Quoted/unquoted delimiters, tab stripping (`<<-`), variable expansion |
| **Subshells** | ✅ Complete | `(...)` grouping with proper state isolation |
| **Traps/Signals** | ✅ Complete | trap builtin, signal handlers, inherited signal propagation, return-in-trap |

### AWK Feature Details

| Feature | Status | Notes |
|---------|--------|-------|
| **Parsing** | ✅ Complete | Full awk grammar |
| **Patterns** | ✅ Complete | BEGIN, END, regex, expressions |
| **Actions** | ✅ Complete | print, printf, assignments |
| **Variables** | ✅ Complete | User vars, fields, special vars |
| **Arrays** | ✅ Complete | Associative arrays, for-in |
| **Control Flow** | ✅ Complete | if/else, while, for, next, break, continue |
| **Regex** | ✅ Complete | Match, substitution, split |
| **Builtins** | ✅ Complete | 30+ functions |
| **printf/sprintf** | ✅ Complete | Format specifiers, width, precision |
| **getline** | ✅ Complete | File, pipe, variable forms |
| **I/O Redirection** | ✅ Complete | `>`, `>>`, `\|` |

**Legend:** 🟢 Complete (>90%) | 🟡 Partial (50-90%) | 🔴 Minimal (<50%) | ❌ Missing

## Quick Start

```bash
# Install toolchain (requires Homebrew)
make setup-toolchain

# Install dev dependencies
make install-dev

# Build native binaries (for testing)
make build

# Build WASM binaries
make build-wasm

# Run tests
make test
```

## Available Utilities

All applets listed above are available in the unified `busybox` multi-call binary (`cmd/busybox`) and can also be built as standalone binaries from `cmd/<applet>`.

Notes:
- `start-stop-daemon` is native-only (excluded from WASM builds).
- Network-facing applets (`wget`, `nc`, `dig`, `ss`) require explicit opt-in when running under WASM.

## Usage

### Native (for development/testing)
```bash
make build
./_build/busybox echo "Hello, World!"
./_build/busybox cat file.txt
./_build/busybox ls -la
```

### WASM (requires wasmtime, wasmer, or similar)
```bash
make build-wasm
wasmtime _build/busybox.wasm echo "Hello, World!"
wasmtime --dir=. _build/busybox.wasm cat file.txt
wasmtime --dir=. _build/busybox.wasm ls -la
```

## Development

### Prerequisites
- Go 1.22+
- TinyGo 0.34+
- Make

### Make Targets

| Target | Description |
|--------|-------------|
| `make help` | Show all targets |
| `make setup-toolchain` | Install Go and TinyGo via brew |
| `make install-dev` | Install linters and security tools |
| `make build` | Build unified busybox (native) |
| `make build-wasm` | Build unified busybox (WASM) |
| `make build-wasm-optimized` | Build size-optimized unified WASM |
| `make test` | Run tests |
| `make coverage` | Run tests with coverage |
| `make lint` | Run golangci-lint |
| `make check` | Run full validation (vet + lint + format) |
| `make clean` | Remove build artifacts |

### Project Structure

```
go-busybox/
├── cmd/                  # Entry points for each applet
│   ├── echo/
│   ├── cat/
│   ├── ls/
│   └── ...
├── pkg/
│   ├── applets/          # Utility implementations
│   │   ├── echo/
│   │   ├── cat/
│   │   └── ...
│   ├── core/             # Shared functionality
│   │   └── fs/           # Sandboxed filesystem operations
│   ├── integration/      # BusyBox comparison tests
│   ├── sandbox/          # Sandboxing and capabilities
│   └── testutil/         # Test helpers
├── testdata/             # Test fixtures
├── _build/               # Build output (gitignored)
├── Makefile
├── SPEC.md               # Detailed specification
└── README.md
```

## Testing

```bash
# Unit tests
make test

# With race detector
make test-race

# With coverage report
make coverage

# Generate HTML coverage
make coverage-html
```

## Sandboxing Model

When running as WASM, utilities operate within WASI's capability-based security model:

- **Filesystem**: Only pre-opened directories are accessible
- **Network**: Disabled by default; network-facing applets require explicit opt-in
- **Memory**: Isolated via WASM linear memory
- **System calls**: Limited to WASI preview1 interface

### Programmatic Sandbox Control

All file operations go through the `pkg/sandbox` package, which can be configured:

```go
import "github.com/rcarmo/go-busybox/pkg/sandbox"

// Initialize sandbox with allowed paths
sandbox.Init(&sandbox.Config{
    AllowedPaths: []sandbox.PathRule{
        {Path: "/data", Permission: sandbox.PermRead | sandbox.PermWrite},
        {Path: "/config", Permission: sandbox.PermRead},
    },
    AllowCwd: true,
    CwdPermission: sandbox.PermRead,
})

// Disable sandbox (for native builds/testing)
sandbox.Disable()
```

Permissions:
- `PermRead` - Read files and directories
- `PermWrite` - Create, modify, delete files
- `PermExec` - Execute files (reserved for future use)

## License

MIT

## Contributing

1. Fork the repository
2. Create a feature branch
3. Run `make check` before committing
4. Run `make security` after vetting and linting (gosec must follow `make check`)
5. Submit a pull request

See [SPEC.md](SPEC.md) for detailed implementation requirements.
