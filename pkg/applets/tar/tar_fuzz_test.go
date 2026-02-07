package tar_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTar(f *testing.F) {
	f.Add([]byte("hi"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"input.txt": string(data)}
		args := []string{"-cf", "archive.tar", "input.txt"}
		testutil.FuzzCompare(t, "tar", tar.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
