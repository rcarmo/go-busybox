package gzip_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/gzip"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzGzip(f *testing.F) {
	f.Add([]byte("hello"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"input.txt": string(data)}
		args := []string{"input.txt"}
		testutil.FuzzCompare(t, "gzip", gzip.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
