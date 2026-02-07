// Package testutil provides shared testing utilities and fixtures.
package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// TempFile creates a temp file with content, returns path.
func TempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TempFileIn creates a temp file in a specific directory.
func TempFileIn(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TempDirWithFiles creates a temp directory populated with files.
// The files map keys are relative paths, values are file contents.
func TempDirWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// CaptureStdio creates a Stdio with captured output buffers.
// Returns the Stdio, stdout buffer, and stderr buffer.
func CaptureStdio(input string) (*core.Stdio, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	return &core.Stdio{
		In:  strings.NewReader(input),
		Out: out,
		Err: errBuf,
	}, out, errBuf
}

// CaptureStdioNoInput creates a Stdio with no input and captured output.
func CaptureStdioNoInput() (*core.Stdio, *bytes.Buffer, *bytes.Buffer) {
	return CaptureStdio("")
}

// AssertExitCode checks that the exit code matches expected.
func AssertExitCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
}

// AssertOutput checks that stdout matches expected.
func AssertOutput(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// AssertOutputContains checks that stdout contains expected substring.
func AssertOutputContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
}

// AssertNoError fails if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// AssertError fails if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// AssertFileExists checks that a file exists.
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("file %s does not exist", path)
	}
}

// AssertFileNotExists checks that a file does not exist.
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("file %s should not exist", path)
	}
}

// AssertFileContent checks that a file contains expected content.
func AssertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("file %s content = %q, want %q", path, got, want)
	}
}

// RunApplet is a helper type for running applet tests.
type RunApplet func(stdio *core.Stdio, args []string) int

// AppletTestCase defines a parameterized test case for applets.
type AppletTestCase struct {
	Name       string                         // Test name
	Args       []string                       // Command line arguments
	Input      string                         // Stdin input
	WantCode   int                            // Expected exit code
	WantOut    string                         // Expected stdout (exact match)
	WantOutSub string                         // Expected stdout substring
	WantErr    string                         // Expected stderr substring
	Files      map[string]string              // Files to create in temp dir
	Setup      func(t *testing.T, dir string) // Optional setup function
	Check      func(t *testing.T, dir string) // Optional post-run check
}

// CaptureAndRun runs an applet with captured stdio and returns the output buffers.
func CaptureAndRun(t *testing.T, run RunApplet, args []string, input string) (*bytes.Buffer, *bytes.Buffer, int) {
	t.Helper()
	stdio, out, errBuf := CaptureStdio(input)
	code := run(stdio, args)
	return out, errBuf, code
}

// RunAppletTests runs a slice of parameterized applet test cases.
func RunAppletTests(t *testing.T, run RunApplet, tests []AppletTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			// Create temp directory with files
			var dir string
			if len(tt.Files) > 0 {
				dir = TempDirWithFiles(t, tt.Files)
			} else {
				dir = t.TempDir()
			}

			// Change to temp dir for relative path tests
			oldDir, _ := os.Getwd()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(oldDir) })

			// Run optional setup
			if tt.Setup != nil {
				tt.Setup(t, dir)
			}

			// Capture stdio
			stdio, out, errBuf := CaptureStdio(tt.Input)

			// Run applet
			code := run(stdio, tt.Args)

			// Check exit code
			AssertExitCode(t, code, tt.WantCode)

			// Check stdout
			if tt.WantOut != "" {
				AssertOutput(t, out.String(), tt.WantOut)
			}
			if tt.WantOutSub != "" {
				AssertOutputContains(t, out.String(), tt.WantOutSub)
			}

			// Check stderr
			if tt.WantErr != "" {
				AssertOutputContains(t, errBuf.String(), tt.WantErr)
			}

			// Run optional post-check
			if tt.Check != nil {
				tt.Check(t, dir)
			}
		})
	}
}
