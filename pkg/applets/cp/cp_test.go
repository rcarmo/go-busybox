package cp_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rcarmo/go-busybox/pkg/applets/cp"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
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
		{
			Name:     "preserve_timestamps",
			Args:     []string{"-p", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a",
			},
			Setup: func(t *testing.T, dir string) {
				older := time.Now().Add(-48 * time.Hour)
				if err := os.Chtimes(filepath.Join(dir, "a.txt"), older, older); err != nil {
					t.Fatal(err)
				}
			},
			Check: func(t *testing.T, dir string) {
				srcInfo, err := os.Stat(filepath.Join(dir, "a.txt"))
				if err != nil {
					t.Fatal(err)
				}
				dstInfo, err := os.Stat(filepath.Join(dir, "b.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if !dstInfo.ModTime().Equal(srcInfo.ModTime()) {
					t.Fatalf("expected preserved mtime, got %v want %v", dstInfo.ModTime(), srcInfo.ModTime())
				}
			},
		},
	}

	testutil.RunAppletTests(t, cp.Run, tests)
}
