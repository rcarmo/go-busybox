# Skill: Testing Strategy & Best Practices

## Goal
Provide a comprehensive, maintainable test suite that maximizes coverage while minimizing duplication through parameterization, fixtures, and continuous code quality monitoring. Prefer comparative testing patterns defined in `.github/skills/comparison/SKILL.md` for parity cases.

## Core Principles

### 1. Parameterized Tests (Table-Driven)
**Always prefer table-driven tests over repetitive test functions.**

```go
// ✅ GOOD: Parameterized
func TestEcho(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        expected string
    }{
        {"empty", []string{}, "\n"},
        {"single", []string{"hello"}, "hello\n"},
        {"multiple", []string{"a", "b"}, "a b\n"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic using tt.args, tt.expected
        })
    }
}

// ❌ BAD: Repetitive
func TestEchoEmpty(t *testing.T) { ... }
func TestEchoSingle(t *testing.T) { ... }
func TestEchoMultiple(t *testing.T) { ... }
```

### 2. Fixtures & Test Helpers
**Extract common setup into reusable fixtures.**

```go
// pkg/testutil/fixtures.go
package testutil

import (
    "bytes"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/rcarmo/go-busybox/pkg/core"
)

// TempFile creates a temp file with content, returns path
func TempFile(t *testing.T, name, content string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), name)
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }
    return path
}

// TempDirWithFiles creates a temp directory with files
func TempDirWithFiles(t *testing.T, files map[string]string) string {
    t.Helper()
    dir := t.TempDir()
    for name, content := range files {
        path := filepath.Join(dir, name)
        _ = os.MkdirAll(filepath.Dir(path), 0755)
        _ = os.WriteFile(path, []byte(content), 0644)
    }
    return dir
}

// CaptureStdio creates a test Stdio with captured output
func CaptureStdio(input string) (*core.Stdio, *bytes.Buffer, *bytes.Buffer) {
    out, errBuf := &bytes.Buffer{}, &bytes.Buffer{}
    return &core.Stdio{
        In:  strings.NewReader(input),
        Out: out,
        Err: errBuf,
    }, out, errBuf
}
```

### 3. Avoid Test Duplication

**Signs of duplication to watch for:**
- Copy-pasted test setup code
- Similar assertions in multiple tests
- Same file/directory creation patterns
- Repeated mock configurations

**Solutions:**
- Extract to `testutil` package
- Use table-driven tests
- Create builder patterns for complex fixtures
- Use `t.Helper()` for helper functions

### 4. Test Organization

```
pkg/
├── applets/
│   └── cat/
│       ├── cat.go
│       └── cat_test.go      # Unit tests alongside code
├── testutil/                 # Shared test utilities
│   ├── fixtures.go
│   ├── assertions.go
│   └── mocks.go
└── integration/              # Integration/E2E tests
    └── applet_test.go
```

### 5. Coverage Requirements

- **Minimum**: 70% line coverage
- **Target**: 85%+ for core packages
- **Critical paths**: 100% (sandbox, security)

```bash
# Check coverage
make coverage
go tool cover -func=coverage.out | grep total

# Find untested code
go tool cover -html=coverage.out
```

## Test Categories

### Unit Tests
- Test individual functions in isolation
- Mock external dependencies
- Fast execution (<100ms per test)

### Integration Tests
- Test applets end-to-end
- Use real filesystem (via t.TempDir())
- Compare output with expected results

### Comparative Tests (Busybox-specific)
- Compare output with real busybox binary
- Fuzz inputs to find divergences
- Document intentional differences

```go
func TestCatVsBusybox(t *testing.T) {
    if _, err := exec.LookPath("busybox"); err != nil {
        t.Skip("busybox not available")
    }
    
    tests := []struct{
        name  string
        args  []string
        input string
    }{
        {"basic", []string{"file.txt"}, ""},
        {"stdin", []string{"-"}, "hello\n"},
        {"number", []string{"-n", "file.txt"}, ""},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Run both, compare output
            ourOutput := runOurCat(tt.args, tt.input)
            busyboxOutput := runBusybox("cat", tt.args, tt.input)
            
            if ourOutput != busyboxOutput {
                t.Errorf("output mismatch:\nours: %q\nbusybox: %q", 
                    ourOutput, busyboxOutput)
            }
        })
    }
}
```

## Continuous Quality Monitoring

### Duplicate Detection
Run periodically to find copy-paste code:

```bash
# Install dupl
go install github.com/mibk/dupl@latest

# Find duplicates (threshold: 50 tokens)
dupl -t 50 ./pkg/...
```

Add to Makefile:
```makefile
.PHONY: dupl
dupl: ## Find duplicate code
	@dupl -t 50 ./pkg/... || true
```

### Test Quality Checks

```makefile
.PHONY: test-quality
test-quality: ## Check test quality metrics
	@echo "=== Coverage ==="
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1
	@echo "\n=== Duplicate Code ==="
	@dupl -t 50 ./pkg/... 2>/dev/null | head -20 || echo "No duplicates found"
	@echo "\n=== Test Count ==="
	@go test -v ./... 2>&1 | grep -c "=== RUN" || echo "0"
```

## Anti-Patterns to Avoid

1. **Testing implementation, not behavior**
   - Test what the function does, not how

2. **Brittle assertions**
   - Don't assert on exact error messages
   - Use `errors.Is()` for error checking

3. **Test interdependence**
   - Each test must be independent
   - Use `t.Parallel()` where safe

4. **Ignoring edge cases**
   - Empty input, nil values
   - Boundary conditions
   - Permission errors

5. **Missing cleanup**
   - Always use `t.TempDir()` (auto-cleanup)
   - Use `t.Cleanup()` for resources

## Make Targets

```makefile
.PHONY: test test-race test-short coverage test-quality dupl

test: ## Run all tests
	@go test -v ./...

test-race: ## Run tests with race detector
	@go test -v -race ./...

test-short: ## Run short tests only
	@go test -v -short ./...

coverage: ## Run tests with coverage
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out

dupl: ## Find duplicate code
	@dupl -t 50 ./pkg/... || true

test-quality: ## Check test quality
	@$(MAKE) coverage
	@echo "Checking for duplicate code..."
	@dupl -t 50 ./pkg/... || true
```

## CI Integration

Tests should run on every PR:

```yaml
# .github/workflows/ci.yml
- name: Run tests
  run: make test-race

- name: Check coverage
  run: |
    make coverage
    COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
    if (( $(echo "$COVERAGE < 70" | bc -l) )); then
      echo "Coverage $COVERAGE% is below 70% threshold"
      exit 1
    fi

- name: Check duplicates
  run: |
    go install github.com/mibk/dupl@latest
    DUPL=$(dupl -t 50 ./pkg/... | wc -l)
    if [ "$DUPL" -gt 0 ]; then
      echo "Found duplicate code:"
      dupl -t 50 ./pkg/...
    fi
```
