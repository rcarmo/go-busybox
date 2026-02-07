package startstopdaemon_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/startstopdaemon"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestStartStopDaemon(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:       "basic",
			Args:       []string{"--start", "--exec", "echo", "--", "ok"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "\n",
		},
	}
	testutil.RunAppletTests(t, startstopdaemon.Run, tests)
}
