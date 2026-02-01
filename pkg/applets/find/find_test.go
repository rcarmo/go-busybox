package find_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/find"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestFind(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "default_print",
			Args:     []string{"."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
				"b.txt": "b",
			},
			Check: func(t *testing.T, dir string) {
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := find.Run(stdio, []string{dir})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "a.txt") || !strings.Contains(out.String(), "b.txt") {
					t.Fatalf("expected files in output, got %q", out.String())
				}
			},
		},
		{
			Name:     "name_filter",
			Args:     []string{"-name", "*.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
				"b.md":  "b",
			},
			Check: func(t *testing.T, dir string) {
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := find.Run(stdio, []string{dir, "-name", "*.txt"})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if strings.Contains(out.String(), "b.md") {
					t.Fatalf("unexpected file in output: %q", out.String())
				}
				if !strings.Contains(out.String(), "a.txt") {
					t.Fatalf("expected a.txt in output: %q", out.String())
				}
			},
		},
		{
			Name:     "type_dir",
			Args:     []string{"-type", "d"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := find.Run(stdio, []string{dir, "-type", "d"})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "subdir") {
					t.Fatalf("expected subdir in output: %q", out.String())
				}
			},
		},
		{
			Name:     "maxdepth",
			Args:     []string{"-maxdepth", "1"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "a/b"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := find.Run(stdio, []string{dir, "-maxdepth", "1"})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if strings.Contains(out.String(), "a/b") {
					t.Fatalf("unexpected depth in output: %q", out.String())
				}
			},
		},
		{
			Name:     "mindepth",
			Args:     []string{"-mindepth", "1"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "a/b"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := find.Run(stdio, []string{dir, "-mindepth", "1"})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				rootPrinted := false
				for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
					if strings.TrimSpace(l) == dir {
						rootPrinted = true
					}
				}
				if rootPrinted {
					t.Fatalf("unexpected root in output: %q", out.String())
				}
			},
		},
	}

	testutil.RunAppletTests(t, find.Run, tests)
}
