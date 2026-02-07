package gzip_test

import (
	"os"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/gzip"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestGzip(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "hello\n",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/input.txt.gz")
				_, err := os.Stat(dir + "/input.txt")
				if err == nil {
					t.Fatalf("expected input.txt to be removed")
				}
			},
		},
	}
	testutil.RunAppletTests(t, gzip.Run, tests)
}
