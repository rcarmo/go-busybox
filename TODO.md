# TODO: BusyBox parity for implemented applets

This checklist targets full behavioral parity (outputs, exit codes, errors, flags, edge cases) for the applets currently implemented in this repo.

When going through this checklist, ensure maxiumum code reuse and proper use of test parametrization and fixtures.

## Cross-cutting parity work (applies to all applets)
- [ ] Pin and document BusyBox reference version (build + source), and capture exact build flags.
- [ ] Expand parity harness to cover: stdin vs file inputs, multiple files, empty files, large files, non-UTF8 bytes, CRLF, missing trailing newline, and binary data.
- [ ] Verify exit codes, stderr wording, and usage text match BusyBox for invalid args across all applets.
- [ ] Add error-path parity tests (permission denied, missing file/dir, broken symlink, directory passed where file expected).
- [ ] Confirm sandbox FS behavior matches BusyBox for symlink traversal, permissions, and cwd changes.
- [ ] Add WASM parity runs (same test matrix) and document any intentional deviations.
- [ ] Add fuzz/differential tests for argv + stdin permutations (bounded).

## echo
- [ ] Match BusyBox `-n`, `-e`, `-E` behavior and escape set (\a \b \c \f \n \r \t \v \\).
- [ ] Validate handling of `--` and unknown flags.

## cat
- [x] Implement `-n`, `-b`, `-A`, `-e`, `-t`, `-v` and verify line numbering/parsing.
- [x] Handle multiple files and stdin (`-`) exactly as BusyBox.
- [ ] Remaining: `-s` not supported by BusyBox; ensure usage text parity (done).

## ls
- [ ] Implement `-a`, `-A`, `-l`, `-h`, `-F`, `-R`, `-t`, `-S`, `-r`.
- [ ] Match column formatting and sorting rules (locale/collation considerations).
- [ ] Ensure symlink targets and file types render correctly in `-l`.

## cp
- [ ] Implement `-r/-R`, `-a`, `-p`, `-f`, `-i`, `-n`, `-v`.
- [ ] Handle directory copying semantics, existing targets, and symlink handling.
- [ ] Preserve permissions/timestamps when requested.

## mv
- [ ] Implement `-f`, `-i`, `-n`, `-v` and overwrite semantics.
- [ ] Match cross-device move behavior (copy + remove) and error messages.

## rm
- [ ] Implement `-r/-R`, `-f`, `-i`, `-v`.
- [ ] Match behavior for directories, non-empty dirs, and missing files.

## head
- [ ] Implement `-c` bytes, `-q/-v`, `-n` with `+N` semantics.
- [ ] Match multi-file headers and error messages.

## tail
- [ ] Implement `-c` bytes, `-q/-v`, `-n` with `+N` semantics.
- [ ] Match multi-file headers and error messages.

## wc
- [ ] Implement `-l`, `-w`, `-c`, `-m` with correct precedence.
- [ ] Match multi-file totals output formatting.

## find
- [ ] Add predicates: `-name/-iname`, `-type`, `-maxdepth/-mindepth` (confirm edge cases), `-print0`.
- [ ] Implement `-path`, `-prune`, `-mtime/-atime/-ctime`, `-size`.
- [ ] Implement actions: `-print`, `-exec`, `-delete` (if permitted in sandbox).

## mkdir
- [ ] Implement `-p`, `-m`, `-v` and mode parsing.

## pwd
- [ ] Implement `-L`/`-P` semantics and symlink resolution parity.

## rmdir
- [x] Implement `-p`, `-v` and error wording parity.

## sort
- [ ] Implement `-r`, `-n`, `-k`, `-t`, `-u`, `-o`, `-f` and stable sort parity.
- [ ] Match locale/byte order behavior (document deviations).

## uniq
- [ ] Implement `-c`, `-d`, `-u`, `-i`, `-f`, `-s` and field/char skipping.

## cut
- [ ] Implement `-d`, `-f` with ranges (`1-3,5`), `-c`, `-b`, and `--output-delimiter`.
- [ ] Match behavior when delimiter missing in line and `-s` (suppress) is set.

## grep
- [x] Implement regex (BRE/ERE), `-E`, `-F`, `-o`, `-r`.
- [x] Implement `-i`, `-v`, `-c`, `-l`, `-H`, `-h`, `-q` basics (regex via Go).
- [x] Support multiple files and correct prefixes.

## sed
- [ ] Implement `-n`, `-e`, `-f`, address ranges, and more commands (d,p,s,g,i,a,c).
- [ ] Match basic regex behavior and escape rules.

## tr
- [ ] Implement character classes, ranges, complement (`-c`), delete (`-d`), squeeze (`-s`).
- [ ] Match handling of mismatched set lengths.

## diff
- [x] Implement default (unified), unified, and brief outputs (BusyBox unified-only).
- [x] Support directories (-r), missing files, binary detection, and exit codes (0/1/2) parity.
- [ ] Support additional BusyBox flags: -a -b -B -d -i -L -N -S -T -t -U -w (if required).

