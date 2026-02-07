package sed_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sed"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzSed(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"s/a/b/", "input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "sed", sed.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
