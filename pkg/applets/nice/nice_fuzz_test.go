package nice_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/nice"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzNice(f *testing.F) {
	f.Add([]byte("0"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		value := testutil.ClampString(string(data), 8)
		if value == "" {
			value = "0"
		}
		args := []string{"-n", value, "echo"}
		testutil.FuzzCompare(t, "nice", nice.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
