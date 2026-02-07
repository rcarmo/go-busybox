package startstopdaemon_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/startstopdaemon"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzStartStopDaemon(f *testing.F) {
	f.Add([]byte("echo"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		cmd := testutil.ClampString(string(data), 16)
		if cmd == "" {
			cmd = "echo"
		}
		args := []string{"--start", "--exec", cmd, "--", "ok"}
		testutil.FuzzCompare(t, "start-stop-daemon", startstopdaemon.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
