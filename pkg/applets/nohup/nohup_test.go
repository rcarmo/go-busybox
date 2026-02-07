package nohup_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nohup"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestNohup(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"echo", "ok"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, nohup.Run, tests)
}
