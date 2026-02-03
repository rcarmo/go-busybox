package wc_test

import (
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/wc"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestWc(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "default_file",
			Args:     []string{"input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "        1         2         4 input.txt\n",
			Files: map[string]string{
				"input.txt": "a b\n",
			},
		},
		{
			Name:     "chars_only",
			Args:     []string{"-m", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "4 input.txt\n",
			Files: map[string]string{
				"input.txt": "a b\n",
			},
		},
		{
			Name:     "bytes_only",
			Args:     []string{"-c", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "4 input.txt\n",
			Files: map[string]string{
				"input.txt": "a b\n",
			},
		},
		{
			Name:     "stdin_default",
			Args:     []string{},
			Input:    "a b\n",
			WantCode: core.ExitSuccess,
			WantOut:  "        1         2         4\n",
		},
	}

	testutil.RunAppletTests(t, wc.Run, tests)
}
