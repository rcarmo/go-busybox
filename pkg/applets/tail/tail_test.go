package tail_test

import (
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/tail"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestTail(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "default_lines",
			Args:     []string{"test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nb\nc\n",
			Files: map[string]string{
				"test.txt": "a\nb\nc\n",
			},
		},
		{
			Name:     "limit_lines",
			Args:     []string{"-n", "2", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b\nc\n",
			Files: map[string]string{
				"test.txt": "a\nb\nc\n",
			},
		},
		{
			Name:     "byte_count",
			Args:     []string{"-c", "3", "bytes.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "def",
			Files: map[string]string{
				"bytes.txt": "abcdef",
			},
		},
		{
			Name:     "multi_header",
			Args:     []string{"file1.txt", "file2.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "==> file1.txt <==\na\nb\n\n==> file2.txt <==\nc\n",
			Files: map[string]string{
				"file1.txt": "a\nb\n",
				"file2.txt": "c\n",
			},
		},
		{
			Name:     "quiet_mode",
			Args:     []string{"-q", "file1.txt", "file2.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nb\nc\n",
			Files: map[string]string{
				"file1.txt": "a\nb\n",
				"file2.txt": "c\n",
			},
		},
		{
			Name:     "missing_file",
			Args:     []string{"/missing"},
			WantCode: core.ExitFailure,
			WantErr:  "tail:",
		},
	}

	testutil.RunAppletTests(t, tail.Run, tests)
}
