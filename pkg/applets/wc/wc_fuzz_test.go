package wc_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/wc"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzWc fuzzes wc with random input.
func FuzzWc(f *testing.F) {
	f.Add([]byte("hello world\n"))
	f.Add([]byte(""))
	f.Add([]byte("one two three\nfour five\nsix\n"))
	f.Add([]byte("no newline at end"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("   spaces   "))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "wc", wc.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzWcFlags tests wc with various flag combinations.
func FuzzWcFlags(f *testing.F) {
	f.Add([]byte("hello world\nfoo bar baz\n"))
	f.Add([]byte(""))
	f.Add([]byte("single"))
	f.Add([]byte("very long line here for testing purposes and maximum line length checking\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-l"},     // lines only
		{"-w"},     // words only
		{"-c"},     // bytes only
		{"-L"},     // max line length
		{"-l", "-w"}, // lines + words
		{"-l", "-c"}, // lines + bytes
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, flags := range flagSets {
			testutil.FuzzCompare(t, "wc", wc.Run, flags, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
