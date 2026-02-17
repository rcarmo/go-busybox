package uniq_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/uniq"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzUniq fuzzes uniq with random input lines.
func FuzzUniq(f *testing.F) {
	f.Add([]byte("aaa\naaa\nbbb\nccc\nccc\n"))
	f.Add([]byte(""))
	f.Add([]byte("single\n"))
	f.Add([]byte("a\nb\na\n"))
	f.Add([]byte("dup\ndup\ndup\n"))
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
		testutil.FuzzCompare(t, "uniq", uniq.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzUniqFlags tests uniq with various flag combinations.
func FuzzUniqFlags(f *testing.F) {
	f.Add([]byte("aaa\naaa\nbbb\naaa\nccc\nccc\n"))
	f.Add([]byte("AAA\naaa\nBBB\n"))
	f.Add([]byte(""))
	f.Add([]byte("single\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-c"},     // count
		{"-d"},     // only duplicated
		{"-u"},     // only unique
		{"-i"},     // case insensitive
		{"-c", "-i"}, // count + case insensitive
		{"-d", "-i"}, // duplicated + case insensitive
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, flags := range flagSets {
			testutil.FuzzCompare(t, "uniq", uniq.Run, flags, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
