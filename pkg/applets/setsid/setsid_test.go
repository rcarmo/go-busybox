package setsid_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/setsid"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestSetsid(t *testing.T) {
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
	testutil.RunAppletTests(t, setsid.Run, tests)
}
