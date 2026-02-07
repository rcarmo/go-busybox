package logname_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/logname"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzLogname(f *testing.F) {
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{}
		testutil.FuzzCompare(t, "logname", logname.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
