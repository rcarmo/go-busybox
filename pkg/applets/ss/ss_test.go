package ss_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ss"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestSs(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:       "basic",
			Args:       []string{"-n"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "Netid",
		},
	}

	testutil.RunAppletTests(t, ss.Run, tests)
}
