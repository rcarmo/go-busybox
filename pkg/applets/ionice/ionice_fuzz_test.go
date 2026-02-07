package ionice_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ionice"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzIOnice(f *testing.F) {
	f.Add([]byte("best"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		class := testutil.ClampString(string(data), 8)
		if class == "" {
			class = "best"
		}
		args := []string{class, "0", "echo"}
		testutil.FuzzCompare(t, "ionice", ionice.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
