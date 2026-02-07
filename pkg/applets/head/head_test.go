package head_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/head"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestHead(t *testing.T) {
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
			WantOut:  "a\nb\n",
			Files: map[string]string{
				"test.txt": "a\nb\nc\n",
			},
		},
		{
			Name:     "byte_count",
			Args:     []string{"-c", "4", "bytes.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "abcd",
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
			WantErr:  "head:",
		},
	}

	testutil.RunAppletTests(t, head.Run, tests)
}
