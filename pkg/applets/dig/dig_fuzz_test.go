package dig_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/dig"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzDig(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"example.com"}
		files := map[string]string{}
		testutil.FuzzCompare(t, "dig", dig.Run, args, input, files, testutil.FuzzOptions{SkipBusybox: true})
	})
}
