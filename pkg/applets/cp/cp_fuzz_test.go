package cp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/applets/cp"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzCp fuzzes cp with random file contents.
func FuzzCp(f *testing.F) {
	f.Add([]byte("sample input"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\n"))
	f.Add([]byte("\x00binary\xff"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		input := string(data)
		args := []string{"input.txt", "out.txt"}
		files := map[string]string{
			"input.txt": input,
		}
		testutil.FuzzCompare(t, "cp", cp.Run, args, input, files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzCpVerify tests that cp actually produces an identical copy.
func FuzzCpVerify(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02\x03"))
	f.Add([]byte("multi\nline\ncontent\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		dir := testutil.TempDirWithFiles(t, map[string]string{
			"src.txt": string(data),
		})
		oldDir, _ := os.Getwd()
		_ = os.Chdir(dir)
		defer func() { _ = os.Chdir(oldDir) }()

		stdio, _, _ := testutil.CaptureStdio("")
		code := cp.Run(stdio, []string{"src.txt", "dst.txt"})
		if code != 0 {
			t.Fatalf("cp returned %d", code)
		}
		got, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
		if err != nil {
			t.Fatalf("dst.txt not created: %v", err)
		}
		if string(got) != string(data) {
			t.Fatalf("copy mismatch: got %d bytes, want %d", len(got), len(data))
		}
	})
}

// FuzzCpFlags tests cp with various flag combinations.
func FuzzCpFlags(f *testing.F) {
	f.Add([]byte("data"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	flagSets := [][]string{
		{"src.txt", "dst.txt"},
		{"-f", "src.txt", "dst.txt"},
		{"-v", "src.txt", "dst.txt"},
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"src.txt": string(data)}
		for _, args := range flagSets {
			testutil.FuzzCompare(t, "cp", cp.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
		}
	})
}
