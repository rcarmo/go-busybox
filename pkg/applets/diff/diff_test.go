package diff_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/diff"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
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
			Name:       "labels",
			Args:       []string{"-L", "LEFT", "-L", "RIGHT", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "--- LEFT",
			Files: map[string]string{
				"a.txt": "a\n",
				"b.txt": "b\n",
			},
		},
		{
			Name:       "allow_absent",
			Args:       []string{"-N", "a.txt", "missing.txt"},
			WantCode:   1,
			WantOutSub: "---",
			Files: map[string]string{
				"a.txt": "a\n",
			},
		},
		{
			Name:       "expand_tabs",
			Args:       []string{"-t", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "a        b",
			Files: map[string]string{
				"a.txt": "a\tb\n",
				"b.txt": "a\tc\n",
			},
		},

		{
			Name:       "brief_diff",
			Args:       []string{"-q", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "Files a.txt and b.txt differ",
			Files: map[string]string{
				"a.txt": "a\n",
				"b.txt": "b\n",
			},
		},
		{
			Name:       "same_flag",
			Args:       []string{"-s", "a.txt", "b.txt"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "",
			Files: map[string]string{
				"a.txt": "same\n",
				"b.txt": "same\n",
			},
		},
		{
			Name:       "binary_diff",
			Args:       []string{"a.bin", "b.bin"},
			WantCode:   1,
			WantOutSub: "Binary files",
			Files: map[string]string{
				"a.bin": "a\x00b",
				"b.bin": "a\x00c",
			},
		},
		{
			Name:       "treat_as_text",
			Args:       []string{"-a", "a.bin", "b.bin"},
			WantCode:   1,
			WantOutSub: "@@",
			Files: map[string]string{
				"a.bin": "a\x00b\n",
				"b.bin": "a\x00c\n",
			},
		},
		{
			Name:       "context_lines",
			Args:       []string{"-U", "1", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "@@",
			Files: map[string]string{
				"a.txt": "a\nb\nc\n",
				"b.txt": "a\nX\nc\n",
			},
		},
		{
			Name:       "directory_recursive",
			Args:       []string{"-r", "left", "right"},
			WantCode:   1,
			WantOutSub: "Only in",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/only.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/other.txt", "b\n")
			},
		},
		{
			Name:     "directory_non_recursive",
			Args:     []string{"left", "right"},
			WantCode: core.ExitUsage,
			WantErr:  "Is a directory",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/a.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/a.txt", "b\n")
			},
		},
		{
			Name:       "start_file",
			Args:       []string{"-r", "-S", "b.txt", "left", "right"},
			WantCode:   1,
			WantOutSub: "@@",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/a.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "left/b.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/b.txt", "b\n")
			},
		},

		{
			Name:       "zero_context",
			Args:       []string{"-U", "0", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "@@",
			Files: map[string]string{
				"a.txt": "a\nb\nc\n",
				"b.txt": "a\nX\nc\n",
			},
		},
		{
			Name:       "large_file_diff",
			Args:       []string{"-U", "1", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "@@",
			Setup: func(t *testing.T, dir string) {
				var left, right strings.Builder
				for i := 0; i < 200; i++ {
					left.WriteString("line" + strconv.Itoa(i) + "\n")
					right.WriteString("line" + strconv.Itoa(i) + "\n")
				}
				left.WriteString("tail\n")
				left.WriteString("end\n")
				right.WriteString("tail\n")
				right.WriteString("fin\n")
				_ = testutil.TempFileIn(t, dir, "a.txt", left.String())
				_ = testutil.TempFileIn(t, dir, "b.txt", right.String())
			},
		},
		{
			Name:       "mixed_binary_recursive",
			Args:       []string{"-r", "left", "right"},
			WantCode:   1,
			WantOutSub: "Binary files",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/bin.dat", "a\x00b")
				_ = testutil.TempFileIn(t, dir, "right/bin.dat", "a\x00c")
				_ = testutil.TempFileIn(t, dir, "left/text.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/text.txt", "b\n")
			},
		},
		{
			Name:       "binary_recursive_text_flag",
			Args:       []string{"-r", "-a", "left", "right"},
			WantCode:   1,
			WantOutSub: "@@",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/bin.dat", "a\x00b\n")
				_ = testutil.TempFileIn(t, dir, "right/bin.dat", "a\x00c\n")
			},
		},
		{
			Name:     "missing_both",
			Args:     []string{"-N", "missing1", "missing2"},
			WantCode: core.ExitUsage,
			WantErr:  "diff:",
		},
		{
			Name:       "missing_left",
			Args:       []string{"-N", "missing.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "+++",
			Files: map[string]string{
				"b.txt": "b\n",
			},
		},

		{
			Name:       "labels_missing",
			Args:       []string{"-L", "LEFT", "-L", "RIGHT", "-N", "missing.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "--- LEFT",
			Files: map[string]string{
				"b.txt": "b\n",
			},
		},
		{
			Name:       "labels_dir_diff",
			Args:       []string{"-r", "-L", "LEFT", "-L", "RIGHT", "left", "right"},
			WantCode:   1,
			WantOutSub: "--- LEFT",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/a.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/a.txt", "b\n")
			},
		},
		{
			Name:       "nested_dir_diff",
			Args:       []string{"-r", "left", "right"},
			WantCode:   1,
			WantOutSub: "@@",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/dir/a.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/dir/a.txt", "b\n")
			},
		},
		{
			Name:     "ignore_blank_diff",
			Args:     []string{"-B", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "a\n\n\n",
				"b.txt": "a\n",
			},
		},
		{
			Name:     "ignore_space_case_combo",
			Args:     []string{"-b", "-i", "a.txt", "b.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"a.txt": "A  B\n",
				"b.txt": "a b\n",
			},
		},
		{
			Name:       "tab_prefix_combo",
			Args:       []string{"-t", "-T", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "\t-",
			Files: map[string]string{
				"a.txt": "a\tb\n",
				"b.txt": "a\tc\n",
			},
		},
		{
			Name:       "recursive_identical",
			Args:       []string{"-r", "-s", "left", "right"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "",
			Setup: func(t *testing.T, dir string) {
				_ = testutil.TempFileIn(t, dir, "left/a.txt", "a\n")
				_ = testutil.TempFileIn(t, dir, "right/a.txt", "a\n")
			},
		},
		{
			Name:       "missing_right",
			Args:       []string{"-N", "a.txt", "missing.txt"},
			WantCode:   1,
			WantOutSub: "---",
			Files: map[string]string{
				"a.txt": "a\n",
			},
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z", "a.txt", "b.txt"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
		{
			Name:       "prefix_tabs",
			Args:       []string{"-T", "a.txt", "b.txt"},
			WantCode:   1,
			WantOutSub: "\t-",
			Files: map[string]string{
				"a.txt": "a\tb\n",
				"b.txt": "a\tc\n",
			},
		},
	}

	testutil.RunAppletTests(t, diff.Run, tests)
}
