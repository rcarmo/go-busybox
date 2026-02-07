package time_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/time"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestTime(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:       "basic",
			Args:       []string{"echo", "hello"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "real",
		},
	}
	testutil.RunAppletTests(t, time.Run, tests)
}
