// Package mv implements the mv command.
package mv

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fileutil"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Options holds mv command options.
type Options struct {
	Force       bool // -f: force overwrite
	Interactive bool // -i: prompt before overwrite
	NoClobber   bool // -n: do not overwrite existing files
	Verbose     bool // -v: verbose output
	MoveToDir   string
	NoTargetDir bool
}

// Run executes the mv command with the given arguments.
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
			case "-t":
				if i+1 >= len(args) {
					return core.UsageError(stdio, "mv", "missing operand")
				}
				i++
				opts.MoveToDir = args[i]
			case "-T":
				opts.NoTargetDir = true
			default:
				for _, c := range arg[1:] {
					switch c {
					case 'f':
						opts.Force = true
					case 'i':
						opts.Interactive = true
					case 'n':
						opts.NoClobber = true
					case 'v':
						opts.Verbose = true
					default:
						return core.UsageError(stdio, "mv", "invalid option -- '"+string(c)+"'")
					}
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}

	if len(paths) < 2 {
		return core.UsageError(stdio, "mv", "missing file operand")
	}

	dest := paths[len(paths)-1]
	sources := paths[:len(paths)-1]
	if opts.MoveToDir != "" {
		sources = paths
		dest = opts.MoveToDir
	}

	dest, destIsDir, code := fileutil.ResolveDest(sources, dest)
	if code != core.ExitSuccess {
		return core.UsageError(stdio, "mv", "target '"+dest+"' is not a directory")
	}
	if opts.NoTargetDir && destIsDir {
		return core.UsageError(stdio, "mv", "target '"+dest+"' is a directory")
	}

	exitCode := core.ExitSuccess
	for _, src := range sources {
		target := fileutil.TargetPath(src, dest, destIsDir)

		if err := movePath(stdio, src, target, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func movePath(stdio *core.Stdio, src, dest string, opts *Options) error {
	if os.Getenv("MV_FORCE_COPY") == "1" {
		if err := copyThenRemove(stdio, src, dest); err != nil {
			return err
		}
		if opts.Verbose {
			stdio.Printf("'%s' -> '%s'\n", src, dest)
		}
		return nil
	}
	// Check if source exists
	if _, err := fs.Stat(src); err != nil {
		stdio.Errorf("mv: cannot stat '%s': %v\n", src, err)
		return err
	}

	// Check if destination exists
	if _, err := fs.Stat(dest); err == nil {
		if opts.NoClobber {
			return nil
		}
		if opts.Interactive && !opts.Force {
			return nil
		}
		if opts.Force {
			_ = fs.Remove(dest)
		}
	}

	// Try rename first (works for same filesystem)
	if err := fs.Rename(src, dest); err != nil {
		if !isCrossDevice(err) {
			stdio.Errorf("mv: cannot move '%s' to '%s': %v\n", src, dest, err)
			return err
		}
		if err := copyThenRemove(stdio, src, dest); err != nil {
			return err
		}
	}

	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}

	return nil
}

func isCrossDevice(err error) bool {
	if linkErr, ok := err.(*os.LinkError); ok {
		return linkErr.Err == os.ErrInvalid || linkErr.Err == os.ErrPermission
	}
	return false
}

func copyThenRemove(stdio *core.Stdio, src, dest string) error {
	info, err := fs.Stat(src)
	if err != nil {
		stdio.Errorf("mv: cannot stat '%s': %v\n", src, err)
		return err
	}
	if info.IsDir() {
		if err := fs.CopyDir(src, dest, true); err != nil {
			stdio.Errorf("mv: cannot copy '%s' to '%s': %v\n", src, dest, err)
			return err
		}
		if err := fs.RemoveAll(src); err != nil {
			stdio.Errorf("mv: cannot remove '%s': %v\n", src, err)
			return err
		}
		return nil
	}
	if err := fs.CopyFile(src, dest, true); err != nil {
		stdio.Errorf("mv: cannot copy '%s' to '%s': %v\n", src, dest, err)
		return err
	}
	if err := fs.Remove(src); err != nil {
		stdio.Errorf("mv: cannot remove '%s': %v\n", src, err)
		return err
	}
	return nil
}
