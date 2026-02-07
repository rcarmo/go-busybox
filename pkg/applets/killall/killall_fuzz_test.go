package killall_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/killall"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzKillall(f *testing.F) {
	f.Add([]byte("bash"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		pattern := testutil.ClampString(string(data), 32)
		if pattern == "" {
			pattern = "bash"
		}
		args := []string{pattern}
		testutil.FuzzCompare(t, "killall", killall.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
