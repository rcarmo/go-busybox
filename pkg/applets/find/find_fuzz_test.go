package find_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/find"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzFind fuzzes find with random file contents in a temp directory.
// Uses SkipBusybox because directory traversal order is implementation-defined.
func FuzzFind(f *testing.F) {
	f.Add([]byte("sample"))
	f.Add([]byte(""))
	f.Add([]byte("test.txt"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, 256)
		files := map[string]string{
			"file.txt":     string(data),
			"sub/deep.txt": "nested",
		}
		args := []string{"."}
		testutil.FuzzCompare(t, "find", find.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzFindName tests find -name with fuzzed patterns.
func FuzzFindName(f *testing.F) {
	f.Add("*.txt")
	f.Add("file*")
	f.Add("*")
	f.Add("*.go")
	f.Add("no-match")
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, pattern string) {
		pattern = testutil.ClampString(pattern, 64)
		files := map[string]string{
			"file.txt": "content",
			"other.go": "code",
			"sub/deep.txt": "nested",
		}
		args := []string{".", "-name", pattern}
		testutil.FuzzCompare(t, "find", find.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzFindFlags tests find with various predicate combinations.
func FuzzFindFlags(f *testing.F) {
	f.Add([]byte("content"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{".", "-type", "f"},
		{".", "-type", "d"},
		{".", "-name", "*.txt"},
		{".", "-name", "*.txt", "-type", "f"},
		{".", "-maxdepth", "1"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, 128)
		files := map[string]string{
			"file.txt":     string(data),
			"other.go":     "code",
			"sub/deep.txt": "nested",
		}
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "find", find.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
