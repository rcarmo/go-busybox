package taskset_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/taskset"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTaskset(f *testing.F) {
	f.Add([]byte("1"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		mask := testutil.ClampString(string(data), 8)
		if mask == "" {
			mask = "1"
		}
		args := []string{mask, "echo"}
		testutil.FuzzCompare(t, "taskset", taskset.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
