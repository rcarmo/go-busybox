package gunzip_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/gunzip"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzGunzip(f *testing.F) {
	f.Add([]byte("notgzip"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"input.gz": string(data)}
		args := []string{"input.gz"}
		testutil.FuzzCompare(t, "gunzip", gunzip.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
