package nohup_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nohup"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzNohup(f *testing.F) {
	f.Add([]byte("echo"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		cmd := testutil.ClampString(string(data), 16)
		if cmd == "" {
			cmd = "echo"
		}
		args := []string{cmd, "ok"}
		testutil.FuzzCompare(t, "nohup", nohup.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
