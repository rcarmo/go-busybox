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
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
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
