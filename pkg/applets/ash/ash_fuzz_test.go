package ash_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ash"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzAsh(f *testing.F) {
	f.Add([]byte("echo ok"))
	f.Add([]byte("echo ok | cat"))
	f.Add([]byte("echo ok > out.txt"))
	f.Add([]byte("for x in a b; do echo $x; done"))
	f.Add([]byte("while true; do echo ok; break; done"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		cmd := testutil.ClampString(string(data), 64)
		if cmd == "" {
			cmd = "echo ok"
		}
		args := []string{cmd}
		testutil.FuzzCompare(t, "ash", ash.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
