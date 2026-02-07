package mv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/mv"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
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
		{
			Name:     "cross_device_fallback",
			Args:     []string{"a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Setup: func(t *testing.T, dir string) {
				_ = os.Setenv("MV_FORCE_COPY", "1")
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a.txt"))
				testutil.AssertFileContent(t, filepath.Join(dir, "b.txt"), "a")
				_ = os.Unsetenv("MV_FORCE_COPY")
			},
		},
	}

	testutil.RunAppletTests(t, mv.Run, tests)
}
