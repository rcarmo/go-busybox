# go-busybox

<p align="center">
  <img src="docs/icon-256.png" alt="Busybox WASM" width="256" />
</p>

Busybox utilities in Go, compilable to WebAssembly via TinyGo for sandboxed execution in AI agents. Produces a unified multi-call `busybox` binary and standalone applet binaries under `cmd/`.

**Reference target:** BusyBox v1.35.0 (Debian 1:1.35.0-4+b7)
**Test suite: 387/387 (100%)**

---

## Contents

- [Quick start](#quick-start)
- [Applets](#applets)
- [Test results](#test-results)
- [Shell](#shell-ash)
- [Text processing](#text-processing)
- [Testing](#testing)
- [Structure](#structure)
- [Sandboxing](#sandboxing)
- [Development](#development)

---

## Quick start

```bash
make setup-toolchain   # Go + TinyGo via Homebrew
make install-dev       # linters, security tools
make build             # native binary → _build/busybox
make build-wasm        # WASM binary       → _build/busybox.wasm  (4.7MB)
make build-wasm-optimized  # size-optimised → _build/busybox.wasm  (2.0MB)
make test
```

```bash
./_build/busybox sed 's/foo/bar/g' input.txt
ln -s _build/busybox _build/grep && ./_build/grep -i pattern file.txt
wasmtime --dir=. _build/busybox.wasm cat file.txt
```

---

## Applets

57 applets, all with full Godoc (including flag lists).

| Category | Applets |
|---|---|
| Shell | `ash` (`sh`) |
| Text | `awk` `cat` `cut` `diff` `echo` `grep` `head` `printf` `sed` `sort` `tail` `tr` `uniq` `wc` |
| Files | `cp` `find` `ls` `mkdir` `mv` `pwd` `rm` `rmdir` |
| Archive | `gunzip` `gzip` `tar` |
| Process | `kill` `killall` `nice` `nohup` `pgrep` `pidof` `pkill` `ps` `renice` `setsid` `start-stop-daemon` `taskset` `time` `timeout` `top` `xargs` |
| System | `free` `ionice` `logname` `nproc` `sleep` `uptime` `users` `w` `watch` `who` `whoami` |
| Network | `dig` `nc` `ss` `wget` |

`start-stop-daemon`, `top`, and 14 other OS-dependent applets return stubs under WASM (see [Sandboxing](#sandboxing)).

---

## Test results

### BusyBox reference suite — 387/387

Runs via `/bin/bash` with `ECHO="echo"`.

**New-style (`.tests`) — 308/308:**

| Applet | Pass | Applet | Pass | Applet | Pass |
|---|---|---|---|---|---|
| awk | 53/53 | gunzip | 5/5 | sort | 5/5 |
| cp | 13/13 | head | 2/2 | tail | 2/2 |
| cut | 22/22 | pidof | 3/3 | tar | 3/3 |
| diff | 12/12 | printf | 24/24 | taskset | 3/3 |
| find | 2/2 | sed | 92/92 | tr | 2/2 |
| grep | 44/44 | | | uniq | 14/14 |
| | | | | xargs | 7/7 |

**Old-style (directory-based) — 79/79:** cat, cp, cut, echo, find, gzip, ls, mkdir, mv, pwd, rm, rmdir, tail, tr, wc, wget.

### ash — 349/349

| Category | Pass | Category | Pass |
|---|---|---|---|
| ash-alias | 5/5 | ash-quoting | 24/24 |
| ash-arith | 6/6 | ash-read | 10/10 |
| ash-comm | 3/3 | ash-redir | 27/27 |
| ash-getopts | 8/8 | ash-signals | 22/22 |
| ash-glob | 10/10 | ash-standalone | 6/6 |
| ash-heredoc | 25/25 | ash-vars | 69/69 |
| ash-invert | 3/3 | ash-z_slow | 3/3 |
| ash-misc | 99/99 | | |
| ash-parsing | 35/35 | **Total** | **349/349** |

---

## Shell (ash)

Near-complete POSIX shell. Pipelines, redirections (`<` `>` `>>` `2>` `>&` `<&` `>&-`), control flow (`if`/`elif`/`else`/`fi`, `while`, `until`, `for`, `case`/`esac`), command substitution (`$(...)`, backticks), arithmetic (`$(())`), functions, parameter expansion (`${VAR:-default}`, `${#VAR}`, `${VAR##pattern}`), 25+ builtins, background jobs (`&`, `jobs`/`fg`/`wait`), here-documents, subshells, traps/signals.

21 fixes applied to reach 349/349:

- **Parser:** backslash-newline continuation; `if` across continuation lines; empty-word handling (`$empty""`); `"$@"` with no args; `if()` reserved-word detection; case-pattern quoting; alias in case body.
- **Expansion:** prefix `IFS=` for builtins; `${x#'*'}` pattern quoting in heredocs; single-quote patterns; backtick inside single quotes; command-sub newline stripping; `unset -ff`; `\<newline>` at EOF.
- **Redirections:** `exec 1>&-` persistence; `1>&-`/`2>&-` parsing; `exec <file` + `read` + `cat`.
- **Signals:** trap exit codes; nested traps; `return` in trap handlers; subshell signal reset.

---

## Text processing

### sed — 92/92

BRE/ERE (`-E`/`-r`), backreferences (`\1`–`\9`), all standard commands (`s` `d` `p` `q` `a` `i` `c` `y` `=` `l` `r` `w` `h` `H` `g` `G` `x` `N` `P` `D` `b` `t` `T`), addresses (line, `$`, regex, ranges, `first~step`, `0,/regex/`), in-place editing (`-i`), escape sequences (`\n` `\t` `\r`), labels/branches, multiple expressions (`-e`, `-f`), `--version`.

Per-line trailing-newline tracking via `noNLLines map[int]bool` — suppresses final `\n` when the last output line originated from an input line lacking a trailing newline. The `w` command truncates output files similarly via `wfileLastLine`.

### diff — 12/12

`-u` `-U N` `-b` `-w` `-B` `-a` `-q` `-r` `-N`. The `-B` implementation post-filters blank-only hunks rather than stripping blanks pre-diff.

### grep — 44/44

`-E` `-F` `-i` `-v` `-c` `-l` `-L` `-n` `-h` `-H` `-o` `-q` `-r` `-w` `-x` `-s` `-e` `-f`. Invokable as `egrep`/`fgrep`.

### awk — 53/53

Full parser/evaluator: BEGIN/END, patterns, arrays, control flow, 30+ builtins, printf/sprintf, getline, regex, I/O redirection.

---

## Testing

Four layers:

1. **Unit tests** — 62 `*_test.go` files, per-applet.
2. **Fuzz tests** — 100 functions across 58 `*_fuzz_test.go` files, 353 seed entries.
3. **Integration** — BusyBox comparison matrix (`pkg/integration/`).
4. **Reference suite** — 387/387.

### Fuzz coverage

Every applet has at least one fuzz test. The 22 most complex have multi-function coverage:

| Applet | Fns | Seeds | Scope |
|---|---|---|---|
| sed | 4 | 33 | input, expression parsing, multi-command, flags |
| grep | 3 | 22 | input, pattern parsing, flags |
| cut | 4 | 20 | delimited fields, field specs, char mode, flags |
| tr | 5 | 22 | translate, delete, squeeze, complement, SET operands |
| diff | 5 | 15 | identical, different, flags, binary, context |
| awk | 2 | 25 | programme parsing, input with fixed programmes |
| tar | 3 | 10 | create, create→extract roundtrip, invalid archives |
| gzip | 3 | 11 | compress, compress→decompress roundtrip, levels |
| gunzip | 3 | 10 | decompress, invalid data, stdin |
| cat | 3 | 11 | file, stdin, multi-file |
| head | 3 | 14 | fixed `-n`, varied `-n`, stdin |
| tail | 3 | 11 | fixed `-n`, stdin, flags |
| sort | 2 | 10 | input, flags (`-r` `-n` `-u` `-f`) |
| uniq | 2 | 9 | input, flags (`-c` `-d` `-u` `-i`) |
| wc | 2 | 10 | input, flags (`-l` `-w` `-c` `-L`) |
| echo | 2 | 10 | arguments, flags (`-n` `-e` `-ne` `-E`) |
| find | 3 | 9 | traversal, `-name` patterns, predicates |
| ls | 2 | 4 | listing, flags (`-1` `-a` `-l` `-R` `-S` `-r` `-t`) |
| cp | 3 | 10 | copy, copy-verify roundtrip, flags |
| xargs | 3 | 11 | echo, no-command default, flags (`-n` `-I`) |
| printf | 2 | 20 | format string, arguments |
| procutil | 2 | 8 | signal parsing, UID lookup |

**Roundtrip tests** verify data integrity:
- `gzip -c | gunzip` — byte-for-byte via `compress/gzip`
- `tar -cf | tar -xf` — extracted files match originals
- `cp src dst` — destination matches source

**Safety filters** prevent non-termination during fuzzing:
- sed: rejects branch labels, `N;D` cycles, excessive quantifiers
- printf: skips format strings that consume no arguments
- All inputs clamped to 2048 bytes

### Running tests

```bash
make test                   # unit tests
make test-race              # with race detector
make coverage               # with coverage

# fuzz seeds only (quick)
go test ./pkg/applets/sed -run='Fuzz' -count=1

# actual fuzzing (10s)
go test ./pkg/applets/sed -run='^$' -fuzz='^FuzzSedExpr$' -fuzztime=10s

# all fuzz seeds
for d in pkg/applets/*/; do go test ./$d -run='Fuzz' -count=1 -timeout 30s; done
```

---

## Structure

```
go-busybox/
├── cmd/                  # standalone entry points (51 + busybox)
│   └── busybox/          # multi-call dispatcher
├── pkg/
│   ├── applets/          # 57 applet packages
│   │   ├── ash/          # shell (5,700+ lines)
│   │   ├── sed/          # stream editor (1,200+ lines)
│   │   ├── procutil/     # shared process helpers
│   │   └── ...
│   ├── core/             # shared: fs/, archiveutil/, headtail, textutil
│   ├── integration/      # BusyBox comparison tests
│   ├── sandbox/          # capability-based sandboxing
│   └── testutil/         # FuzzCompare, CaptureStdio, etc.
├── busybox-reference/    # reference binary (v1.35.0) + test suite
├── docs/SPEC.md
├── plan.md               # ash fix log (21 fixes, all applied)
└── Makefile
```

| Metric | Count |
|---|---|
| Applet packages | 57 (+procutil) |
| Source files (non-test) | 138 |
| Source lines | ~29,000 |
| Unit test files | 62 |
| Fuzz test files | 58 |
| Fuzz functions | 100 |
| Fuzz seeds | 353 |

---

## Sandboxing

Under WASM, utilities run within WASI's capability model: only pre-opened directories are accessible; network is off by default; memory is isolated via WASM linear memory; syscalls are limited to WASI preview1.

```go
sandbox.Init(&sandbox.Config{
    AllowedPaths: []sandbox.PathRule{
        {Path: "/data", Permission: sandbox.PermRead | sandbox.PermWrite},
        {Path: "/config", Permission: sandbox.PermRead},
    },
    AllowCwd:      true,
    CwdPermission: sandbox.PermRead,
})
```

Applets requiring OS APIs unavailable under WASI have `*_wasm.go` stubs that print an error and exit. 16 applets are stubbed:

`ash`, `ionice`, `nice`, `nohup`, `renice`, `setsid`, `ss`, `start-stop-daemon`, `taskset`, `time`, `timeout`, `top`, `users`, `w`, `watch`, `who`.

The remaining 41 applets compile and run natively under WASM. Process-management applets (`kill`, `killall`, `pgrep`, `pkill`, `pidof`, `ps`) compile but depend on `/proc` at runtime — they'll fail gracefully if the filesystem isn't mounted. The `procutil` signal table is reduced under WASI (no `SIGHUP`, `SIGUSR1`, `SIGUSR2`, `SIGALRM`).

---

## Development

Requires Go 1.22+, TinyGo 0.34+ (WASM), Make.

| Target | Description |
|---|---|
| `make build` | native binary → `_build/busybox` |
| `make build-wasm` | WASM binary → `_build/busybox.wasm` |
| `make test` | unit tests |
| `make test-race` | with race detector |
| `make coverage` | with coverage |
| `make lint` | golangci-lint |
| `make check` | vet + lint + format |
| `make security` | gosec |
| `make fuzz-coverage` | verify fuzz coverage ≥80% |
| `make clean` | remove artefacts |

### Adding an applet

1. `pkg/applets/<name>/<name>.go` — `Run(stdio *core.Stdio, args []string) int`
2. Package Godoc + `Run` Godoc with flag list
3. Register in `cmd/busybox/main.go`
4. `cmd/<name>/main.go` entry point
5. `*_test.go` unit tests
6. `*_fuzz_test.go` fuzz tests
7. `*_wasm.go` stub if needed

### Godoc convention

```go
// Run executes the grep command with the given arguments.
//
// Supported flags:
//
//	-E    extended regular expressions
//	-i    ignore case
//	-v    invert match
//	-c    count matching lines
func Run(stdio *core.Stdio, args []string) int {
```

### Applet status

| Applet | Tests | Key features |
|---|---|---|
| ash | 349/349 | POSIX shell, 25+ builtins, signals, jobs, heredocs |
| awk | 53/53 | full parser, 30+ builtins, arrays, getline, printf |
| sed | 92/92 | BRE/ERE, in-place, hold space, branches, NL tracking |
| grep | 44/44 | ERE/fixed, recursive, word/line, multi-pattern |
| diff | 12/12 | unified, whitespace modes, blank-line ignore, recursive |
| cp | 13/13 | recursive, symlinks, `--parents`, hard links |
| printf | 24/24 | full format specs, `%b`, octal/hex |
| cut | 22/22 | fields, characters, bytes, custom delimiters |
| uniq | 14/14 | count, duplicate, unique, case-insensitive |
| sort | 5/5 | numeric, reverse, unique, key-based |
| tar | 3/3 | create, extract, gzip, stdin, symlinks, hardlinks |
| gunzip | 5/5 | keep, stdout, force, stdin |
| wget | 4/4 | `-q`, `-O`, `-P`, combined flags, auto-mkdir |
| pidof | 3/3 | cmdline matching, self-exclusion |
| taskset | 3/3 | CPU affinity get/set |
| xargs | 7/7 | `-n`, `-I`, no-cmd defaults to echo |
| tr | 2/2 | translate, delete, squeeze, complement, POSIX classes |
| start-stop-daemon | — | `--start`/`--exec`, `--pidfile` (native only, partial) |
| top | — | static listing stub |

All others: complete, no dedicated reference-suite tests.

---

## Licence

MIT

## Contributing

1. Fork, branch, hack.
2. `make check` before committing.
3. `make security` after lint.
4. `go test ./pkg/applets/... -run='Fuzz' -count=1` — all seeds must pass.
5. Pull request.

See [SPEC.md](docs/SPEC.md) for the full specification.
