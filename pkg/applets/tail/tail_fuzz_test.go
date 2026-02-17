package tail_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/tail"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzTail fuzzes tail with a fixed line count and fuzzed input.
func FuzzTail(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\nline4\n"))
	f.Add([]byte(""))
	f.Add([]byte("single line"))
	f.Add([]byte("no newline at end"))
	f.Add([]byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-n", "1", "input.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "tail", tail.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzTailStdin tests tail reading from stdin with fuzzed input.
func FuzzTailStdin(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte(""))
	f.Add([]byte("lots\nof\nlines\nhere\nmore\nthan\nten\nactually\nyes\nindeed\nextra\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-n", "3"}
		testutil.FuzzCompare(t, "tail", tail.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzTailFlags tests tail with various flag combinations.
func FuzzTailFlags(f *testing.F) {
	f.Add([]byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n"))
	f.Add([]byte("single\n"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-n", "1"},
		{"-n", "5"},
		{"-n", "20"},
		{"-n", "+3"},
		{"-c", "10"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "tail", tail.Run, args, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
