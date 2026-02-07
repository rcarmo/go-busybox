package ls_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ls"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestLs(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic_listing",
			Args:     []string{"."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"file1.txt": "a",
				"file2.txt": "b",
			},
			WantOutSub: "file1.txt",
		},
		{
			Name:     "hidden_excluded",
			Args:     []string{"."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				".hidden": "",
				"visible": "",
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if strings.Contains(out.String(), ".hidden") {
					t.Error("hidden file should not be shown without -a")
				}
			},
		},
		{
			Name:     "hidden_included",
			Args:     []string{"-a", "."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				".hidden": "",
				"visible": "",
			},
			WantOutSub: ".hidden",
		},
		{
			Name:     "long_format",
			Args:     []string{"-l", "."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"test.txt": "content",
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{"-l", dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "-rw") {
					t.Errorf("long format should show permissions, got %q", out.String())
				}
			},
		},
		{
			Name:     "classify_dir",
			Args:     []string{"-F", "."},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{"-F", dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "subdir/") {
					t.Errorf("expected directory suffix, got %q", out.String())
				}
			},
		},
		{
			Name:     "dir_slash",
			Args:     []string{"-p", "."},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{"-p", dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "subdir/") {
					t.Errorf("expected directory slash, got %q", out.String())
				}
			},
		},
		{
			Name:     "symlink_long",
			Args:     []string{"-l", "."},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				target := filepath.Join(dir, "target.txt")
				testutil.TempFileIn(t, dir, "target.txt", "a")
				if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{"-l", dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "link ->") {
					t.Errorf("expected symlink target, got %q", out.String())
				}
			},
		},
		{
			Name:     "one_per_line",
			Args:     []string{"-1", "."},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "",
				"b.txt": "",
			},
			Check: func(t *testing.T, dir string) {
				out, _, code := testutil.CaptureAndRun(t, ls.Run, []string{"-1", dir}, "")
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				lines := strings.Split(strings.TrimSpace(out.String()), "\n")
				if len(lines) != 2 {
					t.Errorf("expected 2 lines, got %d: %v", len(lines), lines)
				}
			},
		},
		{
			Name:     "missing_path",
			Args:     []string{"/nonexistent/path"},
			WantCode: core.ExitFailure,
			WantErr:  "ls:",
		},
	}

	testutil.RunAppletTests(t, ls.Run, tests)
}
