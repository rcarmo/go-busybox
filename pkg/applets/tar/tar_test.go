package tar_test

import (
	"archive/tar"
	"bytes"
	"testing"

	tarapplet "github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/testutil"
)

func TestTar(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "create",
			Args:     []string{"-cf", "archive.tar", "input.txt"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt": "hello\n",
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/archive.tar")
			},
		},
		{
			Name:     "extract",
			Args:     []string{"-xf", "archive.tar"},
			WantCode: core.ExitSuccess,
			Setup: func(t *testing.T, dir string) {
				testutil.TempFileIn(t, dir, "archive.tar", buildTarBytes(t))
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/file.txt")
			},
		},
	}
	testutil.RunAppletTests(t, tarapplet.Run, tests)
}

func buildTarBytes(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "file.txt", Mode: 0644, Size: int64(len("hi\n"))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("hi\n")); err != nil {
		t.Fatalf("write data: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.String()
}
