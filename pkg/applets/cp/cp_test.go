package cp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/cp"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestCp(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "copy_file",
			Args:     []string{"a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileContent(t, filepath.Join(dir, "b.txt"), "a")
			},
		},
		{
			Name:     "copy_dir",
			Args:     []string{"-r", "src", "dest"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "src/dir"), 0755); err != nil {
					t.Fatal(err)
				}
				testutil.TempFileIn(t, filepath.Join(dir, "src/dir"), "a.txt", "a")
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileContent(t, filepath.Join(dir, "dest/dir/a.txt"), "a")
			},
		},
		{
			Name:     "no_clobber",
			Args:     []string{"-n", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
				"b.txt": "b",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileContent(t, filepath.Join(dir, "b.txt"), "b")
			},
		},
	}

	testutil.RunAppletTests(t, cp.Run, tests)
}
