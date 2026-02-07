package nice_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nice"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestNice(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid",
			Args:     []string{"-n", "bad", "echo"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid number",
		},
	}
	testutil.RunAppletTests(t, nice.Run, tests)
}
