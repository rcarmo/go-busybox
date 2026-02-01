# Skill: Comparative Testing & Source Parity

## Goal
Provide reliable, repeatable parity checks against upstream implementations while minimizing redundant test code and maximizing reusable fixtures/parameterization.

## Core Principles

### 1. Use Differential (A/B) Testing
Always compare our output/exit code to a trusted reference for identical inputs.

```go
func runPair(t *testing.T, applet string, args []string, input string) (ours, ref testutil.RunResult) {
    t.Helper()
    ours = testutil.RunApplet(t, applet, args, input)
    ref = testutil.RunBusybox(t, applet, args, input)
    return ours, ref
}

func assertParity(t *testing.T, ours, ref testutil.RunResult) {
    t.Helper()
    if ours.Code != ref.Code {
        t.Fatalf("exit mismatch: ours=%d ref=%d", ours.Code, ref.Code)
    }
    if ours.Stdout != ref.Stdout {
        t.Fatalf("stdout mismatch:\nours=%q\nref=%q", ours.Stdout, ref.Stdout)
    }
    if ours.Stderr != ref.Stderr {
        t.Fatalf("stderr mismatch:\nours=%q\nref=%q", ours.Stderr, ref.Stderr)
    }
}
```

### 2. Normalize Only When Required
Normalize output only for known, documented differences (e.g., line ending or platform-specific path separators). Never normalize by default.

```go
func normalize(s string) string {
    return strings.ReplaceAll(s, "\r\n", "\n")
}
```

### 3. Parameterize Test Matrices
Prefer table-driven tests to scale parity scenarios without duplication.

```go
tests := []struct {
    name  string
    args  []string
    input string
}{
    {"stdin", []string{"-"}, "hello\n"},
    {"files", []string{"a.txt", "b.txt"}, ""},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        ours, ref := runPair(t, "cat", tt.args, tt.input)
        assertParity(t, ours, ref)
    })
}
```

### 4. Reuse Fixtures and Helpers
Centralize temp dirs, file creation, and stdio capture in `pkg/testutil`.

- `TempDirWithFiles`
- `CaptureStdio`
- `RunApplet`
- `RunBusybox`

### 5. Source Acquisition Strategy (Reference Parity)
Prefer these sources, in order:

1. **System busybox binary** (fastest parity check).
2. **Pinned upstream source tarball** (documented version).
3. **Vendored snapshot** (only if required for offline use).

Document where the reference came from in tests/README. If you must inspect source, record:
- version
- URL
- checksum (if applicable)

### 6. Granular Error Reporting
Errors must show the exact divergence with context (args, input size, and which stream diverged).

```go
func parityError(t *testing.T, applet string, args []string, ours, ref testutil.RunResult) {
    t.Helper()
    t.Fatalf("%s %v parity failure\nexit: ours=%d ref=%d\nstdout: %q\nstderr: %q",
        applet, args, ours.Code, ref.Code, ours.Stdout, ref.Stderr)
}
```

### 7. Avoid Redundant Tests
If a parity test already covers a case, avoid re-creating a unit test for the exact same behavior unless it isolates a tricky edge case.

### 8. Keep Applet Lists in One Place
Store applet parity matrices in a single test file (e.g., `pkg/integration/busybox_compare_test.go`) to avoid drift.

## Anti-Patterns to Avoid
- Copy-pasted test blocks with only minor changes.
- Golden files without parameterized generation.
- Normalizing output to hide real mismatches.
- Comparing against an unpinned or undocumented reference version.

## Checklist
- [ ] Parity tests are table-driven and share helpers.
- [ ] Reference source/version documented.
- [ ] Only minimal normalization applied and documented.
- [ ] Duplicate test patterns extracted to helpers/fixtures.
- [ ] Divergences reported with full context.
