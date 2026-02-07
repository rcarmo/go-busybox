package cut_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cut"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestCut(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "fields_basic",
			Args:     []string{"-f", "2", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b\n",
			Files: map[string]string{
				"input.txt": "a\tb\tc\n",
			},
		},
		{
			Name:     "fields_range",
			Args:     []string{"-f", "1-2", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\tb\n",
			Files: map[string]string{
				"input.txt": "a\tb\tc\n",
			},
		},
		{
			Name:     "fields_delimiter",
			Args:     []string{"-d", ":", "-f", "2", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b\n",
			Files: map[string]string{
				"input.txt": "a:b:c\n",
			},
		},
		{
			Name:     "fields_output_delimiter",
			Args:     []string{"-d", ":", "-f", "1,3", "--output-delimiter", "|", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a|c\n",
			Files: map[string]string{
				"input.txt": "a:b:c\n",
			},
		},
		{
			Name:     "suppress_no_delimiter",
			Args:     []string{"-s", "-f", "1", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "",
			Files: map[string]string{
				"input.txt": "abc\n",
			},
		},
		{
			Name:     "chars_range",
			Args:     []string{"-c", "2-3", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "bc\n",
			Files: map[string]string{
				"input.txt": "abcd\n",
			},
		},
	}

	testutil.RunAppletTests(t, cut.Run, tests)
}
