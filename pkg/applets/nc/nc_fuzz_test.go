package nc_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nc"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzNc(f *testing.F) {
	f.Add([]byte("localhost"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		host := testutil.ClampString(string(data), 64)
		if host == "" {
			host = "localhost"
		}
		args := []string{host, "1"}
		testutil.FuzzCompare(t, "nc", nc.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
