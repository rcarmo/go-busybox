// Package fs provides filesystem operations that respect sandbox boundaries.
// Applets should use this package instead of direct os calls.
package fs

import (
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/rcarmo/go-busybox/pkg/sandbox"
)

// Open opens a file for reading.
func Open(path string) (*os.File, error) {
	return sandbox.Open(path)
}

// Create creates a file for writing.
func Create(path string) (*os.File, error) {
	return sandbox.Create(path)
}

// OpenFile opens a file with flags.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return sandbox.OpenFile(path, flag, perm)
}

// ReadFile reads an entire file.
func ReadFile(path string) ([]byte, error) {
	return sandbox.ReadFile(path)
}

// WriteFile writes data to a file.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return sandbox.WriteFile(path, data, perm)
}

// Stat returns file info.
func Stat(path string) (os.FileInfo, error) {
	return sandbox.Stat(path)
}

// Lstat returns file info without following symlinks.
func Lstat(path string) (os.FileInfo, error) {
	return sandbox.Lstat(path)
}

// ReadDir reads directory contents.
func ReadDir(path string) ([]fs.DirEntry, error) {
	return sandbox.ReadDir(path)
}

// Mkdir creates a directory.
func Mkdir(path string, perm os.FileMode) error {
	return sandbox.Mkdir(path, perm)
}

// MkdirAll creates a directory and parents.
func MkdirAll(path string, perm os.FileMode) error {
	return sandbox.MkdirAll(path, perm)
}

// Remove removes a file or empty directory.
func Remove(path string) error {
	return sandbox.Remove(path)
}

// RemoveAll removes a path recursively.
func RemoveAll(path string) error {
	return sandbox.RemoveAll(path)
}

// Rename renames a file.
func Rename(oldpath, newpath string) error {
	return sandbox.Rename(oldpath, newpath)
}

// Copy copies a file.
func Copy(src, dst string) error {
	return sandbox.Copy(src, dst)
}

// Getwd returns current working directory.
func Getwd() (string, error) {
	return sandbox.Getwd()
}

// Chdir changes directory.
func Chdir(path string) error {
	return sandbox.Chdir(path)
}

// CopyFile copies a file with mode preservation option.
func CopyFile(src, dst string, preserveMode bool) error {
	srcFile, err := Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	mode := os.FileMode(0644)
	if preserveMode {
		mode = srcInfo.Mode()
	}

	dstFile, err := OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Chtimes updates atime/mtime for a path.
func Chtimes(path string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}

// CopyDir copies a directory recursively.
func CopyDir(src, dst string, preserveMode bool) error {
	srcInfo, err := Stat(src)
	if err != nil {
		return err
	}

	mode := os.FileMode(0755)
	if preserveMode {
		mode = srcInfo.Mode()
	}

	if err := MkdirAll(dst, mode); err != nil {
		return err
	}

	entries, err := ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := src + string(os.PathSeparator) + entry.Name()
		dstPath := dst + string(os.PathSeparator) + entry.Name()

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath, preserveMode); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath, preserveMode); err != nil {
				return err
			}
		}
	}

	return nil
}
