package renice_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/renice"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzRenice(f *testing.F) {
	f.Add([]byte("0"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		value := testutil.ClampString(string(data), 8)
		if value == "" {
			value = "0"
		}
		args := []string{value}
		testutil.FuzzCompare(t, "renice", renice.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
