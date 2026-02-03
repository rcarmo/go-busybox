package diff_test

import (
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/diff"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestDiff(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "ignore_space_amount",
			Args:     []string{"-b", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a  b\n",
				"b.txt": "a b\n",
			},
		},
		{
			Name:     "ignore_all_space",
			Args:     []string{"-w", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a b\n",
				"b.txt": "ab\n",
			},
		},
		{
			Name:     "ignore_blank",
			Args:     []string{"-B", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a\n\n",
				"b.txt": "a\n",
			},
		},
		{
			Name:     "ignore_case",
			Args:     []string{"-i", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "A\n",
				"b.txt": "a\n",
			},
		},
		{
			Name:     "labels",
			Args:     []string{"-L", "LEFT", "-L", "RIGHT", "a.txt", "b.txt"},
			WantCode: 1,
			WantOutSub: "--- LEFT",
			Files: map[string]string{
				"a.txt": "a\n",
				"b.txt": "b\n",
			},
		},
		{
			Name:     "allow_absent",
			Args:     []string{"-N", "a.txt", "missing.txt"},
			WantCode: 1,
			WantOutSub: "---",
			Files: map[string]string{
				"a.txt": "a\n",
			},
		},
		{
			Name:     "expand_tabs",
			Args:     []string{"-t", "a.txt", "b.txt"},
			WantCode: 1,
			WantOutSub: "a        b",
			Files: map[string]string{
				"a.txt": "a\tb\n",
				"b.txt": "a\tc\n",
			},
		},
		{
			Name:     "prefix_tabs",
			Args:     []string{"-T", "a.txt", "b.txt"},
			WantCode: 1,
			WantOutSub: "\t-",
			Files: map[string]string{
				"a.txt": "a\tb\n",
				"b.txt": "a\tc\n",
			},
		},
	}

	testutil.RunAppletTests(t, diff.Run, tests)
}
