package find_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rcarmo/go-busybox/pkg/applets/find"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
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
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir}, "")
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
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-name", "*.txt"}, "")
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
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-type", "d"}, "")
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
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-maxdepth", "1"}, "")
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
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-mindepth", "1"}, "")
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
		{
			Name:     "path_filter",
			Args:     []string{"-path", "*/a/*"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "a/b"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-path", "*/a/*"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), filepath.Join(dir, "a", "b")) && !strings.Contains(out.String(), filepath.ToSlash(filepath.Join(dir, "a", "b"))) {
					t.Fatalf("expected path in output: %q", out.String())
				}
			},
		},
		{
			Name:     "print0",
			Args:     []string{"-print0"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-print0"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "\x00") {
					t.Fatalf("expected NUL terminator, got %q", out.String())
				}
			},
		},
		{
			Name:     "prune",
			Args:     []string{"-path", "*/skip", "-prune"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "skip", "child"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-path", "*/skip", "-prune"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if strings.Contains(out.String(), "child") {
					t.Fatalf("expected pruned output, got %q", out.String())
				}
			},
		},
		{
			Name:     "size_filter",
			Args:     []string{"-size", "+0c"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-size", "+0c"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "a.txt") {
					t.Fatalf("expected size match, got %q", out.String())
				}
			},
		},
		{
			Name:     "mtime_filter",
			Args:     []string{"-mtime", "+0"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"old.txt": "a",
			},
			Setup: func(t *testing.T, dir string) {
				path := filepath.Join(dir, "old.txt")
				older := time.Now().Add(-48 * time.Hour)
				if err := os.Chtimes(path, older, older); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, find.Run, []string{dir, "-mtime", "+0"}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "old.txt") {
					t.Fatalf("expected mtime match, got %q", out.String())
				}
			},
		},
	}

	testutil.RunAppletTests(t, find.Run, tests)
}
