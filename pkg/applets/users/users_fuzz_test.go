package users_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/users"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzUsers(f *testing.F) {
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{}
		testutil.FuzzCompare(t, "users", users.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
