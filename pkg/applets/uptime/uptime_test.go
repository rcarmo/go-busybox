package uptime_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/uptime"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestUptime(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{},
			WantCode:   core.ExitSuccess,
			WantOutSub: "load average:",
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, uptime.Run, tests)
}
