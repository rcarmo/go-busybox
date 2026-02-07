package xargs_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/xargs"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzXargs(f *testing.F) {
	f.Add([]byte("one two"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{"echo"}
		input := testutil.ClampString(string(data), testutil.MaxFuzzBytes)
		testutil.FuzzCompare(t, "xargs", xargs.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
