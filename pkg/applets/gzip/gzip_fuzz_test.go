package gzip_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	gzipApplet "github.com/rcarmo/go-busybox/pkg/applets/gzip"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzGzip fuzzes gzip compression with random file contents.
func FuzzGzip(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte(""))
	f.Add([]byte("repeated repeated repeated repeated"))
	f.Add([]byte("\x00\x01\x02\x03"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		files := map[string]string{"input.txt": string(data)}
		args := []string{"input.txt"}
		testutil.FuzzCompare(t, "gzip", gzipApplet.Run, args, "", files, testutil.FuzzOptions{SharedDir: true, SkipBusybox: true})
	})
}

// FuzzGzipRoundtrip tests that gzip -c | gunzip produces the original data.
func FuzzGzipRoundtrip(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte("aaaaaaaaaa"))
	f.Add([]byte("\x00\xff\x80"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)

		// Compress via gzip -c
		dir := t.TempDir()
		inPath := filepath.Join(dir, "input.txt")
		if err := os.WriteFile(inPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		var compBuf bytes.Buffer
		stdio := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: &compBuf,
			Err: io.Discard,
		}
		oldDir, _ := os.Getwd()
		_ = os.Chdir(dir)
		code := gzipApplet.Run(stdio, []string{"-c", "input.txt"})
		_ = os.Chdir(oldDir)
		if code != 0 {
			t.Fatalf("gzip -c returned %d", code)
		}

		// Verify the compressed output is valid gzip
		gr, err := gzip.NewReader(bytes.NewReader(compBuf.Bytes()))
		if err != nil {
			t.Fatalf("gzip output not valid gzip: %v", err)
		}
		decompressed, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("failed to decompress: %v", err)
		}
		gr.Close()

		if !bytes.Equal(decompressed, data) {
			t.Fatalf("roundtrip mismatch: got %d bytes, want %d bytes", len(decompressed), len(data))
		}
	})
}

// FuzzGzipLevels tests gzip at different compression levels.
func FuzzGzipLevels(f *testing.F) {
	f.Add([]byte("compressible data compressible data compressible data"))
	f.Add([]byte("short"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		levels := []string{"-1", "-5", "-9"}
		for _, level := range levels {
			dir := t.TempDir()
			inPath := filepath.Join(dir, "input.txt")
			if err := os.WriteFile(inPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			var compBuf bytes.Buffer
			stdio := &core.Stdio{
				In:  bytes.NewReader(nil),
				Out: &compBuf,
				Err: io.Discard,
			}
			oldDir, _ := os.Getwd()
			_ = os.Chdir(dir)
			code := gzipApplet.Run(stdio, []string{"-c", level, "input.txt"})
			_ = os.Chdir(oldDir)
			if code != 0 {
				t.Fatalf("gzip -c %s returned %d", level, code)
			}
			// Verify decompressible
			gr, err := gzip.NewReader(bytes.NewReader(compBuf.Bytes()))
			if err != nil {
				t.Fatalf("gzip %s output not valid: %v", level, err)
			}
			decompressed, err := io.ReadAll(gr)
			if err != nil {
				t.Fatalf("gzip %s decompress failed: %v", level, err)
			}
			gr.Close()
			if !bytes.Equal(decompressed, data) {
				t.Fatalf("gzip %s roundtrip mismatch", level)
			}
		}
	})
}
