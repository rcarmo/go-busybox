package free_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/free"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestFree(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{},
			WantCode:   core.ExitSuccess,
			WantOutSub: "Mem:",
		},
		{
			Name:       "human",
			Args:       []string{"-h"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "Swap:",
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, free.Run, tests)
}
