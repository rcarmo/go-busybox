package taskset_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/taskset"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestTaskset(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_mask",
			Args:     []string{"bad", "echo"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, taskset.Run, tests)
}
