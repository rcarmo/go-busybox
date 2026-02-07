package sleep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sleep"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestSleep(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "missing",
			Args:       []string{},
			WantCode:   core.ExitFailure,
			WantOutSub: "Usage: sleep",
		},
		{
			Name:     "invalid",
			Args:     []string{"1x"},
			WantCode: core.ExitFailure,
			WantErr:  "invalid number",
		},
		{
			Name:     "short",
			Args:     []string{"0.01"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, sleep.Run, tests)
}
