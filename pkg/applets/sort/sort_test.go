package sort_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sort"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestSort(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "default",
			Args:     []string{"input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "10\n2\na\nc\n",
			Files: map[string]string{
				"input.txt": "c\na\n10\n2\n",
			},
		},
		{
			Name:     "numeric",
			Args:     []string{"-n", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nc\n2\n10\n",
			Files: map[string]string{
				"input.txt": "c\na\n10\n2\n",
			},
		},
		{
			Name:     "reverse",
			Args:     []string{"-r", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "c\na\n2\n10\n",
			Files: map[string]string{
				"input.txt": "c\na\n10\n2\n",
			},
		},
		{
			Name:     "unique",
			Args:     []string{"-u", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "10\n2\na\n",
			Files: map[string]string{
				"input.txt": "a\na\n2\n10\n",
			},
		},
		{
			Name:     "ignore_case",
			Args:     []string{"-f", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nA\n",
			Files: map[string]string{
				"input.txt": "a\nA\n",
			},
		},
		{
			Name:     "key_field",
			Args:     []string{"-k", "2", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b 1\na 2\n",
			Files: map[string]string{
				"input.txt": "a 2\nb 1\n",
			},
		},
		{
			Name:     "separator_key",
			Args:     []string{"-t", ":", "-k", "2", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b:1\na:2\n",
			Files: map[string]string{
				"input.txt": "a:2\nb:1\n",
			},
		},
	}

	testutil.RunAppletTests(t, sort.Run, tests)
}
