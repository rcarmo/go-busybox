package whoami_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/whoami"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestWhoami(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic",
			Args:     []string{},
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, whoami.Run, tests)
}
