package xargs_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/xargs"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzXargs fuzzes xargs input with echo command.
func FuzzXargs(f *testing.F) {
	f.Add([]byte("one two"))
	f.Add([]byte(""))
	f.Add([]byte("a\nb\nc\n"))
	f.Add([]byte("  spaced  out  "))
	f.Add([]byte("'quoted arg'"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		args := []string{"echo"}
		input := testutil.ClampString(string(data), testutil.MaxFuzzBytes)
		testutil.FuzzCompare(t, "xargs", xargs.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzXargsNoCmd fuzzes xargs with no command (defaults to echo).
func FuzzXargsNoCmd(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("a b c"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		input := testutil.ClampString(string(data), testutil.MaxFuzzBytes)
		testutil.FuzzCompare(t, "xargs", xargs.Run, nil, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzXargsFlags tests xargs with various flag combinations.
func FuzzXargsFlags(f *testing.F) {
	f.Add([]byte("one\ntwo\nthree\n"))
	f.Add([]byte("a b c d e f g"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-n", "1", "echo"},
		{"-n", "2", "echo"},
		{"-I", "{}", "echo", "item:{}"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		input := testutil.ClampString(string(data), testutil.MaxFuzzBytes)
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "xargs", xargs.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
