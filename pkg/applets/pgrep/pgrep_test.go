package pgrep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestPgrep(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{"bash"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "\n",
		},
		{
			Name:     "missing",
			Args:     []string{"definitely-not-running"},
			WantCode: core.ExitFailure,
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}

	testutil.RunAppletTests(t, pgrep.Run, tests)
}
