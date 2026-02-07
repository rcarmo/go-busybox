package echo_test

import (
	"fmt"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/echo"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzEcho(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{}
		files := map[string]string{}
		_ = fmt.Sprintf("")
		testutil.FuzzCompare(t, "echo", echo.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}
