package cat_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cat"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzCat fuzzes cat with random file contents.
func FuzzCat(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte("no trailing newline"))
	f.Add([]byte("\x00binary\xff"))
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
		testutil.FuzzCompare(t, "cat", cat.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzCatStdin tests cat reading from stdin with fuzzed data.
func FuzzCatStdin(f *testing.F) {
	f.Add([]byte("hello\n"))
	f.Add([]byte(""))
	f.Add([]byte("multiple\nlines\nhere\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		testutil.FuzzCompare(t, "cat", cat.Run, nil, input, nil, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzCatMultiFile tests cat concatenating two fuzzed files.
func FuzzCatMultiFile(f *testing.F) {
	f.Add([]byte("file1"), []byte("file2"))
	f.Add([]byte(""), []byte(""))
	f.Add([]byte("a\n"), []byte("b\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data1, data2 []byte) {
		data1 = testutil.ClampBytes(data1, testutil.MaxFuzzBytes/2)
		data2 = testutil.ClampBytes(data2, testutil.MaxFuzzBytes/2)
		args := []string{"a.txt", "b.txt"}
		files := map[string]string{
			"a.txt": string(data1),
			"b.txt": string(data2),
		}
		testutil.FuzzCompare(t, "cat", cat.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}
