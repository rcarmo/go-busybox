package kill_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/kill"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzKill(f *testing.F) {
	f.Add([]byte("0"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		pid := testutil.ClampString(string(data), 8)
		if pid == "" {
			pid = "0"
		}
		args := []string{"-0", pid}
		testutil.FuzzCompare(t, "kill", kill.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
