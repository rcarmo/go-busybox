// Package rm implements the rm command.
package rm

import (
	"os"
	"path/filepath"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Options holds rm command options.
type Options struct {
	Recursive   bool // -r, -R: remove directories and their contents
	Force       bool // -f: ignore nonexistent files, never prompt
	Interactive bool // -i: prompt before every removal
	Verbose     bool // -v: verbose output
}

// Run executes the rm command with the given arguments.
//
// Supported flags:
//
//	-r, -R    Remove directories and their contents recursively
//	-f        Force: ignore nonexistent files, never prompt
//	-i        Prompt before every removal (not implemented, accepted)
//	-v        Verbose: print each file as it is removed
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}
	var paths []string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if len(arg) > 0 && arg[0] == '-' && len(arg) > 1 {
			for _, c := range arg[1:] {
				switch c {
				case 'r', 'R':
					opts.Recursive = true
				case 'f':
					opts.Force = true
				case 'i':
					opts.Interactive = true
				case 'v':
					opts.Verbose = true
				default:
					return core.UsageError(stdio, "rm", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}

	if len(paths) == 0 {
		if opts.Force {
			return core.ExitSuccess
		}
		return core.UsageError(stdio, "rm", "missing operand")
	}

	exitCode := core.ExitSuccess
	for _, path := range paths {
		if err := removePath(stdio, path, &opts); err != nil {
			if !opts.Force {
				exitCode = core.ExitFailure
			}
		}
	}

	return exitCode
}

func removePath(stdio *core.Stdio, path string, opts *Options) error {
	info, err := fs.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && opts.Force {
			return nil
		}
		stdio.Errorf("rm: cannot remove '%s': %v\n", path, err)
		return err
	}

	if info.IsDir() {
		if !opts.Recursive {
			stdio.Errorf("rm: cannot remove '%s': Is a directory\n", path)
			return os.ErrInvalid
		}

		return removeDir(stdio, path, opts)
	}

	// Remove file
	if err := fs.Remove(path); err != nil {
		stdio.Errorf("rm: cannot remove '%s': %v\n", path, err)
		return err
	}

	if opts.Verbose {
		stdio.Printf("removed '%s'\n", path)
	}

	return nil
}

func removeDir(stdio *core.Stdio, path string, opts *Options) error {
	entries, err := fs.ReadDir(path)
	if err != nil {
		stdio.Errorf("rm: cannot read directory '%s': %v\n", path, err)
		return err
	}

	// Remove contents first
	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if err := removePath(stdio, entryPath, opts); err != nil {
			return err
		}
	}

	// Remove the directory itself
	if err := fs.Remove(path); err != nil {
		stdio.Errorf("rm: cannot remove '%s': %v\n", path, err)
		return err
	}

	if opts.Verbose {
		stdio.Printf("removed directory: '%s'\n", path)
	}

	return nil
}
