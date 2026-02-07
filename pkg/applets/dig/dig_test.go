package dig_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/dig"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestDig(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "usage_missing_name",
			Args:     []string{},
			WantCode: core.ExitUsage,
			WantErr:  "missing name",
		},
		{
			Name:     "invalid_type",
			Args:     []string{"example.com", "BOGUS"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid arguments",
		},
		{
			Name:     "invalid_reverse",
			Args:     []string{"-x", "not-an-ip"},
			WantCode: core.ExitFailure,
			WantErr:  "invalid address",
		},
		{
			Name:     "invalid_combo",
			Args:     []string{"-4", "-6", "example.com"},
			WantCode: core.ExitUsage,
			WantErr:  "cannot combine",
		},
	}

	testutil.RunAppletTests(t, dig.Run, tests)
}
