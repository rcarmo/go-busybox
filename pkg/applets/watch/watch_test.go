package watch_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/watch"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestWatch(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "interval_missing",
			Args:     []string{"-n", "1"},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"echo", "ok"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, watch.Run, tests)
}
