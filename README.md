# Busybox WASM

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
- Shell: `ash` subset (baseline stub implemented)
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
4. Submit a pull request

See [SPEC.md](SPEC.md) for detailed implementation requirements.
