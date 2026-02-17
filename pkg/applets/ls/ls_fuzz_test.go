package ls_test

import (
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/ls"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzLs fuzzes ls with random file contents in a temp directory.
func FuzzLs(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"."}
		files := map[string]string{
			"file.txt": input,
		}
		testutil.FuzzCompare(t, "ls", ls.Run, args, input, files, testutil.FuzzOptions{SharedDir: true})
	})
}

// FuzzLsFlags tests ls with various flag combinations on a populated directory.
func FuzzLsFlags(f *testing.F) {
	f.Add([]byte("data"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"-1", "."},
		{"-a", "."},
		{"-l", "."},
		{"-la", "."},
		{"-R", "."},
		{"-S", "."},
		{"-r", "."},
		{"-t", "."},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, 256)
		files := map[string]string{
			"aaa.txt": string(data),
			"bbb.txt": "other content",
			"sub/nested.txt": "nested",
		}
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "ls", ls.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
