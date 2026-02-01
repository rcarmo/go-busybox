package ls_test

import (
	"strings"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/ls"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
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
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := ls.Run(stdio, []string{dir})
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
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := ls.Run(stdio, []string{"-l", dir})
				testutil.AssertExitCode(t, code, core.ExitSuccess)
				if !strings.Contains(out.String(), "-rw") {
					t.Errorf("long format should show permissions, got %q", out.String())
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
				stdio, out, _ := testutil.CaptureStdioNoInput()
				code := ls.Run(stdio, []string{"-1", dir})
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
