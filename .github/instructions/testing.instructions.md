# Testing instructions

Applies when: writing or modifying tests in Go projects.

## Mandatory: Use Parameterized Tests

All tests with multiple cases MUST use table-driven (parameterized) approach:

```go
func TestFoo(t *testing.T) {
    tests := []struct {
        name string
        // inputs and expected outputs
    }{
        {"case1", ...},
        {"case2", ...},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

## Use testutil Package

For busybox-wasm applet tests, use the `pkg/testutil` helpers:

```go
import "github.com/rcarmo/busybox-wasm/pkg/testutil"

func TestApplet(t *testing.T) {
    tests := []testutil.AppletTestCase{
        {Name: "basic", Args: []string{"arg"}, WantCode: 0, WantOut: "expected\n"},
    }
    testutil.RunAppletTests(t, applet.Run, tests)
}
```

## Coverage Requirements

- Run `make coverage` before committing
- Minimum 70% coverage for new code
- Critical paths (sandbox, security) require 100%

## Duplicate Code

- Run `make dupl` to check for duplicates
- Extract common patterns to shared helpers
- Use `t.Helper()` for test helper functions

## Test Organization

- Tests live alongside code: `foo.go` → `foo_test.go`
- Shared utilities go in `pkg/testutil/`
- Use `t.TempDir()` for filesystem tests (auto-cleanup)
