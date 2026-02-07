package rmdir_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/rmdir"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestRmdir(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "remove_empty",
			Args:     []string{"empty"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.Mkdir(filepath.Join(dir, "empty"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "empty"))
			},
		},
		{
			Name:     "remove_missing",
			Args:     []string{"missing"},
			WantCode: core.ExitFailure,
			WantErr:  "rmdir: '",
		},
		{
			Name:     "verbose",
			Args:     []string{"-v", "empty"},
			WantCode: core.ExitSuccess,
			WantOut:  "rmdir: removing directory, 'empty'\n",
			Setup: func(t *testing.T, dir string) {
				if err := os.Mkdir(filepath.Join(dir, "empty"), 0755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			Name:     "parents",
			Args:     []string{"-p", "a/b/c"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				if err := os.MkdirAll(filepath.Join(dir, "a/b/c"), 0755); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a/b/c"))
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a/b"))
				testutil.AssertFileNotExists(t, filepath.Join(dir, "a"))
			},
		},
	}

	testutil.RunAppletTests(t, rmdir.Run, tests)
}
