package ps_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ps"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzPs(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		args := []string{}
		files := map[string]string{}
		testutil.FuzzCompare(t, "ps", ps.Run, args, string(data), files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
