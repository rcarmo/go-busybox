package tr_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tr"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTr(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"a-z", "A-Z"}
		files := map[string]string{}
		testutil.FuzzCompare(t, "tr", tr.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
