package top_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/top"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzTop(f *testing.F) {
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{}
		testutil.FuzzCompare(t, "top", top.Run, args, string(data), nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
