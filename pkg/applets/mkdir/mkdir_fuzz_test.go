package mkdir_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/mkdir"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzMkdir(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := ""
		args := []string{}
		files := map[string]string{}
		testutil.FuzzCompare(t, "mkdir", mkdir.Run, args, input, files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
