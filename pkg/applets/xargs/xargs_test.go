package xargs_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/xargs"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestXargs(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "no_args_defaults_to_echo",
			Args:     []string{},
			Input:    "hello world",
			WantOut:  "hello world\n",
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "basic",
			Args:     []string{"echo"},
			Input:    "one two",
			WantCode: core.ExitSuccess,
			WantOut:  "one two\n",
		},
	}
	testutil.RunAppletTests(t, xargs.Run, tests)
}
