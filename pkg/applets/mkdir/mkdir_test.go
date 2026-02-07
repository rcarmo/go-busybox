package mkdir_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/mkdir"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestMkdir(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic",
			Args:     []string{"dir"},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, filepath.Join(dir, "dir"))
			},
		},
		{
			Name:     "parents",
			Args:     []string{"-p", "foo/bar"},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, filepath.Join(dir, "foo"))
				testutil.AssertFileExists(t, filepath.Join(dir, "foo", "bar"))
			},
		},
		{
			Name:     "parents_verbose",
			Args:     []string{"-p", "-v", "a/b"},
			WantCode: core.ExitSuccess,
			WantOut:  "created directory: 'a'\ncreated directory: 'a/b'\n",
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, filepath.Join(dir, "a"))
				testutil.AssertFileExists(t, filepath.Join(dir, "a", "b"))
			},
		},
		{
			Name:     "mode",
			Args:     []string{"-m", "700", "secure"},
			WantCode: core.ExitSuccess,
			Check: func(t *testing.T, dir string) {
				info, err := os.Stat(filepath.Join(dir, "secure"))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0700 {
					t.Fatalf("mode = %o", info.Mode().Perm())
				}
			},
		},
	}

	testutil.RunAppletTests(t, mkdir.Run, tests)
}
