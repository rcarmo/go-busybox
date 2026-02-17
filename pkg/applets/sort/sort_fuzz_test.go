package sort_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/sort"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzSort fuzzes sort with random input lines.
func FuzzSort(f *testing.F) {
	f.Add([]byte("cherry\napple\nbanana\n"))
	f.Add([]byte(""))
	f.Add([]byte("zzz\naaa\nmmm\n"))
	f.Add([]byte("same\nsame\nsame\n"))
	f.Add([]byte("3\n1\n2\n"))
	f.Add([]byte("single"))
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
		testutil.FuzzCompare(t, "sort", sort.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzSortFlags tests sort with various flag combinations.
func FuzzSortFlags(f *testing.F) {
	f.Add([]byte("cherry\napple\nbanana\napple\n"))
	f.Add([]byte("10\n2\n1\n20\n3\n"))
	f.Add([]byte("  zebra\n  apple\n"))
	f.Add([]byte("CCC\naaa\nBBB\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-r"},     // reverse
		{"-n"},     // numeric
		{"-u"},     // unique
		{"-f"},     // case-insensitive
		{"-r", "-n"}, // reverse numeric
		{"-r", "-u"}, // reverse unique
		{"-n", "-u"}, // numeric unique
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		for _, flags := range flagSets {
			args := append(flags, "input.txt")
			files := map[string]string{"input.txt": input}
			testutil.FuzzCompare(t, "sort", sort.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
