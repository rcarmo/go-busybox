package who_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/who"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestWho(t *testing.T) {
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
			WantOutSub: "?",
		},
	}
	testutil.RunAppletTests(t, who.Run, tests)
}
