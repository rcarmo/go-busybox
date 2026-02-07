package wget_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/wget"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzWget(f *testing.F) {
	f.Add([]byte("http://example.com"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		url := testutil.ClampString(string(data), 128)
		if url == "" {
			url = "http://example.com"
		}
		args := []string{url}
		testutil.FuzzCompare(t, "wget", wget.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
