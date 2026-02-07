package find_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/find"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzFind(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"."}
		files := map[string]string{
			"file.txt": input,
		}
		testutil.FuzzCompare(t, "find", find.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
