package cat_test

import (
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/cat"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
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
			WantOut:  "     1	a\n\n     2	b\n",
			Files: map[string]string{
				"test.txt": "a\n\nb\n",
			},
		},
		{
			Name:     "show_ends",
			Args:     []string{"-e", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a$\n",
			Files: map[string]string{
				"test.txt": "a\n",
			},
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
			Name:     "show_all",
			Args:     []string{"-A", "test.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a^Ib$\n",
			Files: map[string]string{
				"test.txt": "a\tb\n",
			},
		},
		{
			Name:       "number_lines",
			Args:       []string{"-n", "test.txt"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "1\t",
			Files: map[string]string{
				"test.txt": "a\nb\nc\n",
			},
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
