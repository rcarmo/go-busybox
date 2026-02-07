package ps_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ps"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestPs(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{},
			WantCode:   core.ExitSuccess,
			WantOutSub: "PID",
		},
		{
			Name:       "custom_columns",
			Args:       []string{"-o", "pid,user,comm"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "PID",
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-q"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
		{
			Name:       "ignored_option",
			Args:       []string{"-Z"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "PID",
		},
	}

	testutil.RunAppletTests(t, ps.Run, tests)
}
