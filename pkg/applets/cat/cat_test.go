package cat_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cat"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestCat(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic_file",
			Args:     []string{"test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "hello\nworld\n",
			Files: map[string]string{
				"test.txt": "hello\nworld\n",
			},
		},
		{
			Name:     "multiple_files",
			Args:     []string{"a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "aaa\nbbb\n",
			Files: map[string]string{
				"a.txt": "aaa\n",
				"b.txt": "bbb\n",
			},
		},
		{
			Name:     "number_nonblank",
			Args:     []string{"-b", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "     1\tline 1\n\n     2\tline 3\n",
			Files: map[string]string{
				"test.txt": "line 1\n\nline 3\n",
			},
		},
		{
			Name:     "show_ends",
			Args:     []string{"-e"},
			Input:    "foo\n",
			WantCode: core.ExitSuccess,
			WantOut:  "foo$\n",
		},
		{
			Name:     "show_tabs",
			Args:     []string{"-t", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a^Ib\n",
			Files: map[string]string{
				"test.txt": "a\tb\n",
			},
		},
		{
			Name:     "show_nonprint",
			Args:     []string{"-v"},
			Input:    "foo\n",
			WantCode: core.ExitSuccess,
			WantOut:  "foo\n",
		},
		{
			Name:     "show_all",
			Args:     []string{"-A", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a^Ib$\n",
			Files: map[string]string{
				"test.txt": "a\tb\n",
			},
		},
		{
			Name:     "number_lines",
			Args:     []string{"-n"},
			Input:    "line 1\n\nline 3\n",
			WantCode: core.ExitSuccess,
			WantOut:  "     1\tline 1\n     2\t\n     3\tline 3\n",
		},
		{
			Name:     "stdin",
			Args:     []string{"-"},
			Input:    "stdin content\n",
			WantCode: core.ExitSuccess,
			WantOut:  "stdin content\n",
		},
		{
			Name:     "missing_file",
			Args:     []string{"/nonexistent/file"},
			WantCode: core.ExitFailure,
			WantErr:  "cat:",
		},
	}

	testutil.RunAppletTests(t, cat.Run, tests)
}
