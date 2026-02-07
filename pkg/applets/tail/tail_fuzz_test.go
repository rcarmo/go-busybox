package tail_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tail"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTail(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-n", "1", "input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "tail", tail.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
