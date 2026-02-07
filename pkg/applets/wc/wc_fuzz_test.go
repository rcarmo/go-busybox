package wc_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/wc"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzWc(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "wc", wc.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
