package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rcarmo/go-busybox/pkg/sandbox"
)

func TestSandboxDisabled(t *testing.T) {
	sandbox.Disable()

	// Should allow all operations when disabled
	_, err := sandbox.Stat("/etc/passwd")
	if err != nil {
		t.Errorf("expected no error when sandbox disabled, got %v", err)
	}
}

func TestSandboxEnabled(t *testing.T) {
	dir := t.TempDir()

	err := sandbox.Init(&sandbox.Config{
		AllowedPaths: []sandbox.PathRule{
			{Path: dir, Permission: sandbox.PermRead | sandbox.PermWrite},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Should allow access to allowed path
	testFile := filepath.Join(dir, "test.txt")
	err = sandbox.WriteFile(testFile, []byte("hello"), 0644)
	if err != nil {
		t.Errorf("expected write to succeed in allowed path, got %v", err)
	}

	// Should deny access to other paths
	_, err = sandbox.Stat("/etc/passwd")
	if err != sandbox.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied for /etc/passwd, got %v", err)
	}
}

func TestSandboxReadOnly(t *testing.T) {
	dir := t.TempDir()

	// Create a test file before enabling sandbox
	testFile := filepath.Join(dir, "readonly.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := sandbox.Init(&sandbox.Config{
		AllowedPaths: []sandbox.PathRule{
			{Path: dir, Permission: sandbox.PermRead}, // Read-only
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Should allow read
	_, err = sandbox.ReadFile(testFile)
	if err != nil {
		t.Errorf("expected read to succeed, got %v", err)
	}

	// Should deny write
	err = sandbox.WriteFile(testFile, []byte("new content"), 0644)
	if err != sandbox.ErrReadOnly {
		t.Errorf("expected ErrReadOnly, got %v", err)
	}
}

func TestSandboxCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	err = sandbox.Init(&sandbox.Config{
		AllowCwd:      true,
		CwdPermission: sandbox.PermRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Should allow read in cwd
	_, err = sandbox.ReadDir(cwd)
	if err != nil {
		t.Errorf("expected ReadDir to succeed in cwd, got %v", err)
	}

	// Should deny access outside cwd
	_, err = sandbox.Stat("/etc/passwd")
	if err != sandbox.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied for /etc/passwd, got %v", err)
	}
}

func TestSandboxPathTraversal(t *testing.T) {
	dir := t.TempDir()

	err := sandbox.Init(&sandbox.Config{
		AllowedPaths: []sandbox.PathRule{
			{Path: dir, Permission: sandbox.PermRead | sandbox.PermWrite},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Attempt path traversal
	traversalPath := filepath.Join(dir, "..", "..", "etc", "passwd")
	_, err = sandbox.Stat(traversalPath)
	if err != sandbox.ErrAccessDenied {
		t.Errorf("expected ErrAccessDenied for traversal path, got %v", err)
	}
}

func TestSandboxMkdir(t *testing.T) {
	dir := t.TempDir()

	err := sandbox.Init(&sandbox.Config{
		AllowedPaths: []sandbox.PathRule{
			{Path: dir, Permission: sandbox.PermRead | sandbox.PermWrite},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Should allow mkdir in allowed path
	newDir := filepath.Join(dir, "newdir")
	err = sandbox.Mkdir(newDir, 0755)
	if err != nil {
		t.Errorf("expected mkdir to succeed, got %v", err)
	}

	// Verify directory was created
	info, err := sandbox.Stat(newDir)
	if err != nil {
		t.Errorf("expected stat to succeed, got %v", err)
	}
	if !info.IsDir() {
		t.Error("expected created path to be a directory")
	}
}

func TestSandboxRename(t *testing.T) {
	dir := t.TempDir()

	// Create test file
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := sandbox.Init(&sandbox.Config{
		AllowedPaths: []sandbox.PathRule{
			{Path: dir, Permission: sandbox.PermRead | sandbox.PermWrite},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sandbox.Disable()

	// Should allow rename within sandbox
	dst := filepath.Join(dir, "dst.txt")
	err = sandbox.Rename(src, dst)
	if err != nil {
		t.Errorf("expected rename to succeed, got %v", err)
	}

	// Verify file was renamed
	_, err = sandbox.Stat(dst)
	if err != nil {
		t.Errorf("expected dst to exist, got %v", err)
	}
}
