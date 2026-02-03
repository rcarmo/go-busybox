package mv_test

import (
	"path/filepath"
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/mv"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestMv(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "move_file",
			Args:     []string{"a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a.txt"))
				testutil.AssertFileContent(t, filepath.Join(dir, "b.txt"), "a")
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
				testutil.AssertFileContent(t, filepath.Join(dir, "a.txt"), "a")
			},
		},
	}

	testutil.RunAppletTests(t, mv.Run, tests)
}
