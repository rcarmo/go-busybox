package uniq_test

import (
	"testing"

	"github.com/rcarmo/busybox-wasm/pkg/applets/uniq"
	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/testutil"
)

func TestUniq(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "default",
			Args:     []string{"input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\nb\n",
			Files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			Name:     "count",
			Args:     []string{"-c", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "      2 a\n      1 b\n",
			Files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			Name:     "dup",
			Args:     []string{"-d", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "a\n",
			Files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
		{
			Name:     "uniq",
			Args:     []string{"-u", "input.txt"},
			WantCode: core.ExitSuccess,
			WantOut:  "b\n",
			Files: map[string]string{
				"input.txt": "a\na\nb\n",
			},
		},
	}

	testutil.RunAppletTests(t, uniq.Run, tests)
}
