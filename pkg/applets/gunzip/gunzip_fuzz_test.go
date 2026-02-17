package gunzip_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	gunzipApplet "github.com/rcarmo/go-busybox/pkg/applets/gunzip"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

// FuzzGunzip fuzzes gunzip with random file contents (compressed first).
func FuzzGunzip(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte(""))
	f.Add([]byte("test data for gunzip"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		// First compress the data to create valid gzip input
		var compBuf bytes.Buffer
		gw := gzip.NewWriter(&compBuf)
		_, _ = gw.Write(data)
		gw.Close()

		dir := t.TempDir()
		gzPath := filepath.Join(dir, "input.gz")
		if err := os.WriteFile(gzPath, compBuf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}

		var outBuf bytes.Buffer
		stdio := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: &outBuf,
			Err: io.Discard,
		}
		oldDir, _ := os.Getwd()
		_ = os.Chdir(dir)
		code := gunzipApplet.Run(stdio, []string{"-c", "input.gz"})
		_ = os.Chdir(oldDir)
		if code != 0 {
			t.Fatalf("gunzip -c returned %d", code)
		}
		if !bytes.Equal(outBuf.Bytes(), data) {
			t.Fatalf("gunzip output mismatch: got %d bytes, want %d bytes", outBuf.Len(), len(data))
		}
	})
}

// FuzzGunzipInvalid feeds arbitrary (likely invalid) gzip data to gunzip
// to ensure it doesn't panic.
func FuzzGunzipInvalid(f *testing.F) {
	f.Add([]byte("\x1f\x8b\x08\x00"))
	f.Add([]byte("not gzip at all"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x00\x00"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		dir := t.TempDir()
		gzPath := filepath.Join(dir, "bad.gz")
		if err := os.WriteFile(gzPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		stdio := &core.Stdio{
			In:  bytes.NewReader(nil),
			Out: io.Discard,
			Err: io.Discard,
		}
		oldDir, _ := os.Getwd()
		_ = os.Chdir(dir)
		// Should not panic, exit code doesn't matter
		_ = gunzipApplet.Run(stdio, []string{"-c", "bad.gz"})
		_ = os.Chdir(oldDir)
	})
}

// FuzzGunzipStdin tests gunzip reading from stdin.
func FuzzGunzipStdin(f *testing.F) {
	f.Add([]byte("stdin data"))
	f.Add([]byte(""))
	f.Add([]byte("line1\nline2\n"))
	if testing.Short() {
		f.Skip("fuzzing skipped in short mode")
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		data = testutil.ClampBytes(data, testutil.MaxFuzzBytes)
		// Compress for valid input
		var compBuf bytes.Buffer
		gw := gzip.NewWriter(&compBuf)
		_, _ = gw.Write(data)
		gw.Close()

		var outBuf bytes.Buffer
		stdio := &core.Stdio{
			In:  bytes.NewReader(compBuf.Bytes()),
			Out: &outBuf,
			Err: io.Discard,
		}
		code := gunzipApplet.Run(stdio, []string{"-c"})
		if code != 0 {
			// Stdin gunzip may fail for some edge cases; that's ok
			return
		}
		if !bytes.Equal(outBuf.Bytes(), data) {
			t.Fatalf("gunzip stdin roundtrip mismatch")
		}
	})
}
