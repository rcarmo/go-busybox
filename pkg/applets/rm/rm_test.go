package rm_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/rm"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestRm(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "remove_file",
			Args:     []string{"a.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a.txt"))
			},
		},
		{
			Name:     "remove_dir_recursive",
			Args:     []string{"-r", "dir"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "dir/sub"), 0755); err != nil {
					t.Fatal(err)
				}
				testutil.TempFileIn(t, filepath.Join(dir, "dir/sub"), "file.txt", "a")
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "dir"))
			},
		},
	}

	testutil.RunAppletTests(t, rm.Run, tests)
}
