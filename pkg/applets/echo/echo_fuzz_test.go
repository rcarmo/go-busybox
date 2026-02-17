package echo_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/echo"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzEcho fuzzes echo arguments to test escape sequence handling.
func FuzzEcho(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("-n"))
	f.Add([]byte("-e"))
	f.Add([]byte("\\n\\t\\\\"))
	f.Add([]byte("\\x41\\x42"))
	f.Add([]byte("-ne hello"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		args := []string{string(data)}
		testutil.FuzzCompare(t, "echo", echo.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzEchoFlags tests echo with -n and -e flags and fuzzed content.
func FuzzEchoFlags(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte("a b c"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-n"},
		{"-e"},
		{"-ne"},
		{"-E"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		text := string(data)
		for _, flags := range flagSets {
			args := append(flags, text)
			testutil.FuzzCompare(t, "echo", echo.Run, args, "", nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
