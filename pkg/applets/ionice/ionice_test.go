package ionice_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ionice"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestIOnice(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_class",
			Args:     []string{"bad", "0", "echo"},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_level",
			Args:     []string{"best", "9", "echo"},
			WantCode: core.ExitUsage,
		},
	}
	testutil.RunAppletTests(t, ionice.Run, tests)
}
