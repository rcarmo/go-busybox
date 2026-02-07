package logname_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/logname"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestLogname(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic",
			Args:     []string{},
			WantCode: core.ExitFailure,
			WantErr:  "logname: getlogin",
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, logname.Run, tests)
}
