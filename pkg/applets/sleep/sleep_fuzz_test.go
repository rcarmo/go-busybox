package sleep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sleep"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzSleep(f *testing.F) {
	f.Add([]byte("0"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{"0"}
		testutil.FuzzCompare(t, "sleep", sleep.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
