# Busybox WASM

<p align="center">
  <img src="docs/icon-256.png" alt="Busybox WASM" width="256" />
</p>

A sandboxable implementation of busybox utilities in Go, compiled to WebAssembly using TinyGo.

## Overview

This project ports common busybox utilities to Go, targeting WebAssembly (WASI) for secure, sandboxed execution. It provides:

- **Capability-based sandboxing** via WASM's memory isolation
- **POSIX-compatible utilities** for shell scripting
- **Comparative testing** against the original C busybox binary
- **Small binary sizes** (<100KB per applet, <2MB combined)
- **Envisioned dual use**: a WASM sandboxing tool and a way to extend GoKrazy on embedded devices

## Reference BusyBox

Current parity target: **BusyBox v1.35.0 (Debian 1:1.35.0-4+b7)** as installed on the test host.

## Feature Completeness Status

### Applet Implementation Status

| Category | Applet | Status | Notes |
|----------|--------|--------|-------|
| **Shell** | ash | 🟡 ~85% | Builtins complete; pipelines, redirects, control flow, functions, case/esac, arithmetic, command substitution |
| **Text Processing** | awk | 🟢 ~90% | Full parser/evaluator, builtins, printf/sprintf, getline, regex |
| | sed | 🟢 Complete | Basic and extended regex, in-place editing |
| | grep | 🟢 Complete | -E, -i, -v, -c, -l, -n, -r flags |
| | cut | 🟢 Complete | Fields, characters, delimiters |
| | tr | 🟢 Complete | Character translation and deletion |
| | sort | 🟢 Complete | Numeric, reverse, unique, key-based sorting |
| | uniq | 🟢 Complete | Count, duplicate, unique modes |
| | wc | 🟢 Complete | Lines, words, characters, bytes |
| | diff | 🟢 Complete | Unified diff, context, recursive |
| **File Operations** | cat | 🟢 Complete | Number lines, show ends/tabs |
| | head | 🟢 Complete | Lines and bytes modes |
| | tail | 🟢 Complete | Lines, bytes, follow mode |
| | cp | 🟢 Complete | Recursive, preserve, no-clobber |
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
| **Parsing** | ✅ Complete | Tokenizer, quoting, escapes |
| **Pipelines** | ✅ Complete | Multi-stage with timeout protection |
| **Redirections** | ✅ Complete | `<`, `>`, `>>`, `2>`, `2>>` |
| **Control Flow** | ✅ Complete | if/elif/else/fi, while, for, case/esac |
| **Command Substitution** | ✅ Complete | `$(...)` and backticks |
| **Arithmetic** | ✅ Complete | `$((...))` with operators |
| **Functions** | ✅ Complete | Definition and positional params |
| **Parameter Expansion** | ✅ Complete | `${VAR:-default}`, `${#VAR}`, `${VAR##pattern}`, etc. |
| **Positional Params** | ✅ Complete | `$0`-`$9`, `$@`, `$*`, `$#`, shift |
| **Special Variables** | ✅ Complete | `$$`, `$?`, `$!` |
| **File Tests** | ✅ Complete | -e, -f, -d, -r, -w, -x, -s, -L |
| **Builtins** | ✅ Complete | 25+ builtins including cd, export, eval, read, printf, alias, getopts, trap |
| **Background Jobs** | 🟡 Basic | `&`, jobs/fg/wait with minimal tracking |
| **Here-documents** | 🟡 Partial | Marker detection; content parsing WIP |
| **Subshells** | 🟡 Basic | `(...)` grouping |
| **Traps/Signals** | 🟡 Partial | trap builtin stores handlers; signal wiring pending |

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

### Phase 1 (Foundation)
- `echo` - Display text
- `cat` - Concatenate files
- `ls` - List directory contents
- `cp` - Copy files
- `mv` - Move/rename files
- `rm` - Remove files

### Phase 2 (In Progress)
- File: `head`, `tail`, `wc`
- Directory: `mkdir`, `rmdir`, `pwd`
- Planned: `sort`, `uniq`, `cut`, `grep`, `find`, `sed`, `tr`, `diff` (awk parity via goawk)

### Phase 3 (Planned)
- Shell: `ash` implementation largely complete (job control/traps partial)
- Process: `ps`, `kill`, `xargs`
- Archive: `tar` (tar/gzip/gunzip baseline implemented)
- Network: `wget`, `nc` (sandboxed; wget/nc baseline implemented), `dig`

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
busybox-wasm/
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
│   └── sandbox/          # Sandboxing and capabilities
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
- **Network**: Disabled by default (Phase 3 utilities require explicit opt-in)
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
