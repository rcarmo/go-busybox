package w_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/w"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzW(f *testing.F) {
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{}
		testutil.FuzzCompare(t, "w", w.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
