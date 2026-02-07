package cp_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cp"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzCp(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"input.txt", "out.txt"}
		files := map[string]string{
			"input.txt": input,
			"out.txt":   "data",
		}
		testutil.FuzzCompare(t, "cp", cp.Run, args, input, files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
