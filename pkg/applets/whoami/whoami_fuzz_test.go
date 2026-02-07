package whoami_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/whoami"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzWhoami(f *testing.F) {
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{}
		testutil.FuzzCompare(t, "whoami", whoami.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
