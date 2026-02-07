// Package sandbox provides capability-based filesystem access control.
// It wraps standard file operations and restricts access to pre-authorized paths.
package sandbox

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Common sandbox errors.
var (
	ErrAccessDenied   = errors.New("access denied: path not in sandbox")
	ErrNotInitialized = errors.New("sandbox not initialized")
	ErrReadOnly       = errors.New("write access denied: sandbox is read-only")
)

// Permission represents file access permissions.
type Permission uint8

const (
	PermNone  Permission = 0
	PermRead  Permission = 1 << iota // Can read files
	PermWrite                        // Can write/create files
	PermExec                         // Can execute files (future use)
)

// PathRule defines access rules for a path prefix.
type PathRule struct {
	Path       string     // Path prefix (resolved to absolute)
	Permission Permission // Allowed operations
}

// Sandbox provides controlled filesystem access.
type Sandbox struct {
	mu       sync.RWMutex
	rules    []PathRule
	enabled  bool
	cwd      string // Current working directory within sandbox
	allowCwd bool   // Allow operations in current working directory
}

// Config holds sandbox configuration.
type Config struct {
	// Paths to allow access to (with permissions)
	AllowedPaths []PathRule
	// Allow access to current working directory
	AllowCwd bool
	// Default permission for cwd if AllowCwd is true
	CwdPermission Permission
}

// Global sandbox instance (disabled by default for native builds).
var globalSandbox = &Sandbox{enabled: false}

// Init initializes the global sandbox with the given configuration.
func Init(cfg *Config) error {
	globalSandbox.mu.Lock()
	defer globalSandbox.mu.Unlock()

	globalSandbox.rules = nil
	globalSandbox.enabled = true
	globalSandbox.allowCwd = cfg.AllowCwd

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	globalSandbox.cwd = cwd

	// Add cwd rule if enabled
	if cfg.AllowCwd {
		perm := cfg.CwdPermission
		if perm == PermNone {
			perm = PermRead | PermWrite
		}
		globalSandbox.rules = append(globalSandbox.rules, PathRule{
			Path:       cwd,
			Permission: perm,
		})
	}

	// Add configured paths
	for _, rule := range cfg.AllowedPaths {
		absPath, err := filepath.Abs(rule.Path)
		if err != nil {
			continue
		}
		globalSandbox.rules = append(globalSandbox.rules, PathRule{
			Path:       absPath,
			Permission: rule.Permission,
		})
	}

	return nil
}

// Disable disables the sandbox (allows all operations).
func Disable() {
	globalSandbox.mu.Lock()
	defer globalSandbox.mu.Unlock()
	globalSandbox.enabled = false
}

// Enable enables the sandbox.
func Enable() {
	globalSandbox.mu.Lock()
	defer globalSandbox.mu.Unlock()
	globalSandbox.enabled = true
}

// IsEnabled returns whether the sandbox is enabled.
func IsEnabled() bool {
	globalSandbox.mu.RLock()
	defer globalSandbox.mu.RUnlock()
	return globalSandbox.enabled
}

// checkAccess verifies if the given path can be accessed with the requested permission.
func checkAccess(path string, perm Permission) error {
	globalSandbox.mu.RLock()
	defer globalSandbox.mu.RUnlock()

	if !globalSandbox.enabled {
		return nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ErrAccessDenied
	}

	// Clean the path to prevent traversal attacks
	absPath = filepath.Clean(absPath)

	for _, rule := range globalSandbox.rules {
		if strings.HasPrefix(absPath, rule.Path) || absPath == rule.Path {
			// Check if we're accessing a subdirectory or the exact path
			remainder := strings.TrimPrefix(absPath, rule.Path)
			if remainder == "" || strings.HasPrefix(remainder, string(filepath.Separator)) {
				if rule.Permission&perm == perm {
					return nil
				}
				if perm&PermWrite != 0 && rule.Permission&PermWrite == 0 {
					return ErrReadOnly
				}
			}
		}
	}

	return ErrAccessDenied
}

// Open opens a file for reading within the sandbox.
func Open(path string) (*os.File, error) {
	if err := checkAccess(path, PermRead); err != nil {
		return nil, err
	}
	return os.Open(path) // #nosec G304 -- sandbox checkAccess enforces allowed paths
}

// Create creates a file within the sandbox.
func Create(path string) (*os.File, error) {
	if err := checkAccess(path, PermWrite); err != nil {
		return nil, err
	}
	return os.Create(path) // #nosec G304 -- sandbox checkAccess enforces allowed paths
}

// OpenFile opens a file with the given flags within the sandbox.
func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	required := PermRead
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		required |= PermWrite
	}
	if err := checkAccess(path, required); err != nil {
		return nil, err
	}
	return os.OpenFile(path, flag, perm) // #nosec G304 -- sandbox checkAccess enforces allowed paths
}

// ReadFile reads a file within the sandbox.
func ReadFile(path string) ([]byte, error) {
	if err := checkAccess(path, PermRead); err != nil {
		return nil, err
	}
	return os.ReadFile(path) // #nosec G304 -- sandbox checkAccess enforces allowed paths
}

// WriteFile writes data to a file within the sandbox.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := checkAccess(path, PermWrite); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// Stat returns file info within the sandbox.
func Stat(path string) (os.FileInfo, error) {
	if err := checkAccess(path, PermRead); err != nil {
		return nil, err
	}
	return os.Stat(path)
}

// Lstat returns file info (not following symlinks) within the sandbox.
func Lstat(path string) (os.FileInfo, error) {
	if err := checkAccess(path, PermRead); err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

// ReadDir reads a directory within the sandbox.
func ReadDir(path string) ([]fs.DirEntry, error) {
	if err := checkAccess(path, PermRead); err != nil {
		return nil, err
	}
	return os.ReadDir(path)
}

// Mkdir creates a directory within the sandbox.
func Mkdir(path string, perm os.FileMode) error {
	if err := checkAccess(path, PermWrite); err != nil {
		return err
	}
	return os.Mkdir(path, perm)
}

// MkdirAll creates a directory and parents within the sandbox.
func MkdirAll(path string, perm os.FileMode) error {
	if err := checkAccess(path, PermWrite); err != nil {
		return err
	}
	return os.MkdirAll(path, perm)
}

// Remove removes a file or empty directory within the sandbox.
func Remove(path string) error {
	if err := checkAccess(path, PermWrite); err != nil {
		return err
	}
	return os.Remove(path)
}

// RemoveAll removes a path and all children within the sandbox.
func RemoveAll(path string) error {
	if err := checkAccess(path, PermWrite); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// Rename renames a file within the sandbox.
func Rename(oldpath, newpath string) error {
	if err := checkAccess(oldpath, PermWrite); err != nil {
		return err
	}
	if err := checkAccess(newpath, PermWrite); err != nil {
		return err
	}
	return os.Rename(oldpath, newpath)
}

// Copy copies a file within the sandbox.
func Copy(src, dst string) error {
	if err := checkAccess(src, PermRead); err != nil {
		return err
	}
	if err := checkAccess(dst, PermWrite); err != nil {
		return err
	}

	srcFile, err := os.Open(src) // #nosec G304 -- sandbox checkAccess enforces allowed paths
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode()) // #nosec G304 -- sandbox checkAccess enforces allowed paths
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// Getwd returns the current working directory.
func Getwd() (string, error) {
	return os.Getwd()
}

// Chdir changes the current working directory within sandbox constraints.
func Chdir(path string) error {
	if err := checkAccess(path, PermRead); err != nil {
		return err
	}
	return os.Chdir(path)
}
