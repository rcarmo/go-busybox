// Package cp implements the cp command.
package cp

import (
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fileutil"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Options holds cp command options.
type Options struct {
	Recursive     bool   // -r, -R: copy directories recursively
	Force         bool   // -f: force overwrite
	Interactive   bool   // -i: prompt before overwrite
	Preserve      bool   // -p: preserve mode, ownership, timestamps
	NoClobber     bool   // -n: do not overwrite existing files
	Verbose       bool   // -v: verbose output
	CopyToDir     string
	NoTargetDir   bool
	NoDereference bool // -P, -d: don't follow symlinks
	Dereference   bool // -L: follow all symlinks
	DerefArgs     bool // -H: follow only command-line symlinks
	Parents       bool // --parents: preserve source path structure
	hardLinks     map[hardLinkKey]string // track inode -> dest for hard link preservation
}

// hardLinkKey identifies a file by device+inode for hard link tracking
type hardLinkKey struct {
	dev uint64
	ino uint64
}

// Run executes the cp command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}

	var paths []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if len(arg) > 0 && arg[0] == '-' && len(arg) > 1 {
			switch arg {
			case "--parents":
				opts.Parents = true
				continue
			case "-t":
				if i+1 >= len(args) {
					return core.UsageError(stdio, "cp", "missing operand")
				}
				i++
				opts.CopyToDir = args[i]
			case "-T":
				opts.NoTargetDir = true
			default:
				for _, c := range arg[1:] {
					switch c {
					case 'f':
						opts.Force = true
					case 'i':
						opts.Interactive = true
					case 'p':
						opts.Preserve = true
					case 'n':
						opts.NoClobber = true
					case 'v':
						opts.Verbose = true
					case 'r', 'R':
						opts.Recursive = true
					case 'a':
						opts.Preserve = true
						opts.Recursive = true
						opts.NoDereference = true
					case 'd':
						opts.NoDereference = true
					case 'P':
						opts.NoDereference = true
						opts.Dereference = false
					case 'L':
						opts.Dereference = true
						opts.NoDereference = false
					case 'H':
						opts.DerefArgs = true
					default:
						return core.UsageError(stdio, "cp", "invalid option -- '"+string(c)+"'")
					}
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}

	// POSIX: cp -R without -L defaults to -P (preserve symlinks)
	// -H overrides: follow command-line symlinks, but preserve internal
	if opts.Recursive && !opts.Dereference {
		opts.NoDereference = true
	}

	if len(paths) < 2 {
		return core.UsageError(stdio, "cp", "missing file operand")
	}

	dest := paths[len(paths)-1]
	sources := paths[:len(paths)-1]
	if opts.CopyToDir != "" {
		sources = paths
		dest = opts.CopyToDir
	}

	dest, destIsDir, code := fileutil.ResolveDest(sources, dest)
	if code != core.ExitSuccess {
		return core.UsageError(stdio, "cp", "target '"+dest+"' is not a directory")
	}
	if opts.NoTargetDir && destIsDir {
		return core.UsageError(stdio, "cp", "target '"+dest+"' is a directory")
	}

	// Initialize hard link tracking for -d copies
	if opts.NoDereference {
		opts.hardLinks = make(map[hardLinkKey]string)
	}

	exitCode := core.ExitSuccess
	for _, src := range sources {
		var target string
		if opts.Parents {
			// --parents: replicate source path under dest
			target = filepath.Join(dest, src)
			// Ensure parent directories exist
			parentDir := filepath.Dir(target)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				stdio.Errorf("cp: cannot create directory '%s': %v\n", parentDir, err)
				exitCode = core.ExitFailure
				continue
			}
		} else {
			target = fileutil.TargetPath(src, dest, destIsDir)
		}

		if err := copyPath(stdio, src, target, &opts, true); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func copyPath(stdio *core.Stdio, src, dest string, opts *Options, isTopLevel bool) error {
	// Determine whether to follow symlinks for this source
	var srcInfo os.FileInfo
	var err error
	followSymlink := true
	if opts.NoDereference {
		followSymlink = false
	}
	if opts.DerefArgs && isTopLevel {
		followSymlink = true
	}
	if opts.Dereference {
		followSymlink = true
	}

	if followSymlink {
		srcInfo, err = fs.Stat(src)
	} else {
		srcInfo, err = fs.Lstat(src)
	}
	if err != nil {
		stdio.Errorf("cp: cannot stat '%s': %v\n", src, err)
		return err
	}

	// Handle symlinks when not following them
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return copySymlink(stdio, src, dest, opts)
	}

	if srcInfo.IsDir() {
		if !opts.Recursive {
			stdio.Errorf("cp: omitting directory '%s'\n", src)
			return os.ErrInvalid
		}
		return copyDir(stdio, src, dest, opts)
	}

	return copyFile(stdio, src, dest, srcInfo, opts)
}

func copySymlink(stdio *core.Stdio, src, dest string, opts *Options) error {
	target, err := os.Readlink(src)
	if err != nil {
		stdio.Errorf("cp: cannot read symlink '%s': %v\n", src, err)
		return err
	}
	// Remove destination if it exists
	_ = os.Remove(dest)
	if err := os.Symlink(target, dest); err != nil {
		stdio.Errorf("cp: cannot create symlink '%s': %v\n", dest, err)
		return err
	}
	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}
	return nil
}

func copyFile(stdio *core.Stdio, src, dest string, srcInfo os.FileInfo, opts *Options) error {
	// Check for hard link preservation
	if opts.hardLinks != nil {
		if stat, ok := srcInfo.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
			key := hardLinkKey{dev: stat.Dev, ino: stat.Ino}
			if existingDest, found := opts.hardLinks[key]; found {
				// Create a hard link to the already-copied file
				_ = os.Remove(dest)
				if err := os.Link(existingDest, dest); err == nil {
					if opts.Verbose {
						stdio.Printf("'%s' -> '%s'\n", src, dest)
					}
					return nil
				}
				// Fall through to normal copy if link fails
			} else {
				opts.hardLinks[key] = dest
			}
		}
	}

	// Check if destination exists
	if _, err := fs.Stat(dest); err == nil {
		if opts.NoClobber {
			return nil
		}
		if opts.Interactive && !opts.Force {
			return nil
		}
	}

	srcFile, err := fs.Open(src)
	if err != nil {
		stdio.Errorf("cp: cannot open '%s': %v\n", src, err)
		return err
	}
	defer srcFile.Close()

	mode := srcInfo.Mode()
	if !opts.Preserve {
		mode = 0644
	}

	destFile, err := fs.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		stdio.Errorf("cp: cannot create '%s': %v\n", dest, err)
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		stdio.Errorf("cp: error writing '%s': %v\n", dest, err)
		return err
	}

	if opts.Preserve {
		_ = fs.Chtimes(dest, srcInfo.ModTime(), srcInfo.ModTime())
	}

	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}

	return nil
}

func copyDir(stdio *core.Stdio, src, dest string, opts *Options) error {
	srcInfo, err := fs.Stat(src)
	if err != nil {
		return err
	}

	mode := srcInfo.Mode()
	if !opts.Preserve {
		mode = 0755
	}

	if err := fs.MkdirAll(dest, mode); err != nil {
		stdio.Errorf("cp: cannot create directory '%s': %v\n", dest, err)
		return err
	}

	entries, err := fs.ReadDir(src)
	if err != nil {
		stdio.Errorf("cp: cannot read directory '%s': %v\n", src, err)
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		// Check if entry is a symlink
		entryInfo, err := fs.Lstat(srcPath)
		if err != nil {
			stdio.Errorf("cp: cannot stat '%s': %v\n", srcPath, err)
			return err
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 && opts.NoDereference && !opts.Dereference {
			if err := copySymlink(stdio, srcPath, destPath, opts); err != nil {
				return err
			}
			continue
		}

		if err := copyPath(stdio, srcPath, destPath, opts, false); err != nil {
			return err
		}
	}

	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}

	return nil
}
