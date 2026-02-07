package diff_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/diff"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func FuzzDiff(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
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

func FuzzDiffFlags(f *testing.F) {
	f.Add([]byte("sample"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"-b", "input.txt", "other.txt"}
		files := map[string]string{
			"input.txt": input,
			"other.txt": input + "x",
		}
		testutil.FuzzCompare(t, "diff", diff.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

func FuzzDiffBinary(f *testing.F) {
	f.Add([]byte("binary"))
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

func FuzzDiffLarge(f *testing.F) {
	f.Add([]byte("seed"))
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
