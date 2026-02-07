package gunzip_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/gunzip"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestGunzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("hello\n"))
	_ = zw.Close()

	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"input.txt.gz"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt.gz": buf.String(),
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/input.txt")
			},
		},
	}
	testutil.RunAppletTests(t, gunzip.Run, tests)
}
