package diff_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/diff"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzDiff fuzzes diff with identical files to test the equal-input path.
func FuzzDiff(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\nline3\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"input.txt", "other.txt"}
		files := map[string]string{
			"input.txt": input,
			"other.txt": input,
		}
		testutil.FuzzCompare(t, "diff", diff.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzDiffDifferent fuzzes diff with two independently fuzzed files
// to exercise the actual diff algorithm.
func FuzzDiffDifferent(f *testing.F) {
	f.Add([]byte("aaa\nbbb\nccc\n"), []byte("aaa\nxxx\nccc\n"))
	f.Add([]byte("a\n"), []byte("b\n"))
	f.Add([]byte(""), []byte("new\n"))
	f.Add([]byte("old\n"), []byte(""))
	f.Add([]byte("same\n"), []byte("same\n"))
	f.Add([]byte("a\nb\nc\nd\ne\n"), []byte("a\nc\ne\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data1, data2 []byte) {
		data1 = testutil.ClampBytes(data1, testutil.MaxFuzzBytes)
		data2 = testutil.ClampBytes(data2, testutil.MaxFuzzBytes)
		args := []string{"a.txt", "b.txt"}
		files := map[string]string{
			"a.txt": string(data1),
			"b.txt": string(data2),
		}
		testutil.FuzzCompare(t, "diff", diff.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzDiffFlags tests diff with various flag combinations.
func FuzzDiffFlags(f *testing.F) {
	f.Add([]byte("sample"))
	f.Add([]byte("line1\nline2\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		modified := input + "x"
		flagSets := [][]string{
			{"-u", "a.txt", "b.txt"},
			{"-b", "a.txt", "b.txt"},
			{"-w", "a.txt", "b.txt"},
			{"-B", "a.txt", "b.txt"},
			{"-U", "0", "a.txt", "b.txt"},
			{"-U", "5", "a.txt", "b.txt"},
		}
		for _, args := range flagSets {
			files := map[string]string{
				"a.txt": input,
				"b.txt": modified,
			}
			testutil.FuzzCompare(t, "diff", diff.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}

// FuzzDiffBinary fuzzes diff with binary-like data.
func FuzzDiffBinary(f *testing.F) {
	f.Add([]byte("binary"))
	f.Add([]byte("\x00\x01\x02"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		args := []string{"-a", "input.bin", "other.bin"}
		files := map[string]string{
			"input.bin": string(data),
			"other.bin": string(data) + "x",
		}
		testutil.FuzzCompare(t, "diff", diff.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzDiffLarge tests diff with context-zero unified output on fuzzed data.
func FuzzDiffLarge(f *testing.F) {
	f.Add([]byte("seed"))
	f.Add([]byte("a\nb\nc\nd\ne\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		args := []string{"-U", "0", "a.txt", "b.txt"}
		files := map[string]string{
			"a.txt": string(data) + "\nend\n",
			"b.txt": string(data) + "\nfin\n",
		}
		testutil.FuzzCompare(t, "diff", diff.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
