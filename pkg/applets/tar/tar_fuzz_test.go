package tar_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	tarApplet "github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzTar fuzzes tar archive creation with random file contents.
func FuzzTar(f *testing.F) {
	f.Add([]byte("hi"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\nline3\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"input.txt": string(data)}
		args := []string{"-cf", "archive.tar", "input.txt"}
		testutil.FuzzCompare(t, "tar", tarApplet.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzTarRoundtrip tests that tar cf | tar xf produces the original files.
func FuzzTarRoundtrip(f *testing.F) {
	f.Add([]byte("hello world"), []byte("second file"))
	f.Add([]byte(""), []byte(""))
	f.Add([]byte("line1\nline2\n"), []byte("other\ncontent\n"))
	f.Add([]byte("\x00\xff binary"), []byte("normal text"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data1, data2 []byte) {
		data1 = testutil.ClampBytes(data1, testutil.MaxFuzzBytes/2)
		data2 = testutil.ClampBytes(data2, testutil.MaxFuzzBytes/2)

		// Create source directory
		srcDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), data1, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "file2.txt"), data2, 0600); err != nil {
			t.Fatal(err)
		}

		// Create archive
		oldDir, _ := os.Getwd()
		_ = os.Chdir(srcDir)
		stdio := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: io.Discard,
			Err: io.Discard,
		}
		code := tarApplet.Run(stdio, []string{"-cf", "archive.tar", "file1.txt", "file2.txt"})
		_ = os.Chdir(oldDir)
		if code != 0 {
			t.Fatalf("tar -cf returned %d", code)
		}

		// Extract to new directory
		dstDir := t.TempDir()
		archivePath := filepath.Join(srcDir, "archive.tar")
		archiveData, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, "archive.tar"), archiveData, 0600); err != nil {
			t.Fatal(err)
		}

		_ = os.Chdir(dstDir)
		stdio2 := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: io.Discard,
			Err: io.Discard,
		}
		code = tarApplet.Run(stdio2, []string{"-xf", "archive.tar"})
		_ = os.Chdir(oldDir)
		if code != 0 {
			t.Fatalf("tar -xf returned %d", code)
		}

		// Verify extracted files match
		got1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		if err != nil {
			t.Fatalf("file1.txt not extracted: %v", err)
		}
		got2, err := os.ReadFile(filepath.Join(dstDir, "file2.txt"))
		if err != nil {
			t.Fatalf("file2.txt not extracted: %v", err)
		}
		if !bytes.Equal(got1, data1) {
			t.Fatalf("file1.txt mismatch: got %d bytes, want %d", len(got1), len(data1))
		}
		if !bytes.Equal(got2, data2) {
			t.Fatalf("file2.txt mismatch: got %d bytes, want %d", len(got2), len(data2))
		}
	})
}

// FuzzTarInvalid feeds arbitrary bytes to tar -xf to test that it
// handles invalid archives gracefully without panicking.
func FuzzTarInvalid(f *testing.F) {
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("not a tar file"))
	f.Add([]byte(""))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		dir := t.TempDir()
		archivePath := filepath.Join(dir, "bad.tar")
		if err := os.WriteFile(archivePath, data, 0600); err != nil {
			t.Fatal(err)
		}
		oldDir, _ := os.Getwd()
		_ = os.Chdir(dir)
		stdio := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: io.Discard,
			Err: io.Discard,
		}
		// Should not panic
		_ = tarApplet.Run(stdio, []string{"-xf", "bad.tar"})
		_ = os.Chdir(oldDir)
	})
}
