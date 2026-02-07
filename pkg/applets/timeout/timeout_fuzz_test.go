package timeout_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/timeout"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTimeout(f *testing.F) {
	f.Add([]byte("1"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dur := testutil.ClampString(string(data), 8)
		if dur == "" {
			dur = "1"
		}
		args := []string{dur, "echo", "ok"}
		testutil.FuzzCompare(t, "timeout", timeout.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
