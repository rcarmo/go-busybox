package users_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/users"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestUsers(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, users.Run, tests)
}
