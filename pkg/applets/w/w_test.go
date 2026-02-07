package w_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/w"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestW(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
		},
		{
			Name:       "basic",
			Args:       []string{},
			WantCode:   core.ExitSuccess,
			WantOutSub: "USER",
		},
	}
	testutil.RunAppletTests(t, w.Run, tests)
}
