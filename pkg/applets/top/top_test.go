package top_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/top"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestTop(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{},
			WantCode:   core.ExitSuccess,
			WantOutSub: "PID",
		},
	}
	testutil.RunAppletTests(t, top.Run, tests)
}
