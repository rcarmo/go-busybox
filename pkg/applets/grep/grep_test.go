package grep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/grep"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestGrep(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "usage_error",
			Args:     []string{},
			WantCode: core.ExitUsage,
			WantErr:  "grep:",
		},
		{
			Name:     "exit_success",
			Args:     []string{"grep", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "grep\n",
			Files: map[string]string{
				"input": "grep\n",
			},
		},
		{
			Name:     "stdin_default",
			Args:     []string{"two"},
			Input:    "one\ntwo\nthree\nthree\nthree\n",
			WantCode: core.ExitSuccess,
			WantOut:  "two\n",
		},
		{
			Name:     "stdin_dash",
			Args:     []string{"two", "-"},
			Input:    "one\ntwo\nthree\nthree\nthree\n",
			WantCode: core.ExitSuccess,
			WantOut:  "two\n",
		},
		{
			Name:     "file_input",
			Args:     []string{"two", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "two\n",
			Files: map[string]string{
				"input": "one\ntwo\nthree\nthree\nthree\n",
			},
		},
		{
			Name:     "no_newline",
			Args:     []string{"bug", "input"},
			WantCode: core.ExitSuccess,
			WantOut:  "bug\n",
			Files: map[string]string{
				"input": "bug",
			},
		},
	}

	testutil.RunAppletTests(t, grep.Run, tests)
}
