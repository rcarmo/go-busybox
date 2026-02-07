package grep_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/grep"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzGrep(f *testing.F) {
	f.Add([]byte("match"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		pattern := "match"
		input := string(data)
		args := []string{pattern, "input.txt"}
		files := map[string]string{"input.txt": input}
		testutil.FuzzCompare(t, "grep", grep.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
