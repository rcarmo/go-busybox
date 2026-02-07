package nproc_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nproc"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestNproc(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "basic",
			Args:     []string{},
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "ignore",
			Args:     []string{"--ignore=1"},
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, nproc.Run, tests)
}
