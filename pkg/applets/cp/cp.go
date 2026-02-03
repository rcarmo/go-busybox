// Package cp implements the cp command.
package cp

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fileutil"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Options holds cp command options.
type Options struct {
	Recursive   bool // -r, -R: copy directories recursively
	Force       bool // -f: force overwrite
	Interactive bool // -i: prompt before overwrite
	Preserve    bool // -p: preserve mode, ownership, timestamps
	NoClobber   bool // -n: do not overwrite existing files
	Verbose     bool // -v: verbose output
	CopyToDir   string
	NoTargetDir bool
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
					default:
						return core.UsageError(stdio, "cp", "invalid option -- '"+string(c)+"'")
					}
				}
			}
		} else {
			paths = append(paths, arg)
		}
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

	exitCode := core.ExitSuccess
	for _, src := range sources {
		target := fileutil.TargetPath(src, dest, destIsDir)

		if err := copyPath(stdio, src, target, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func copyPath(stdio *core.Stdio, src, dest string, opts *Options) error {
	srcInfo, err := fs.Stat(src)
	if err != nil {
		stdio.Errorf("cp: cannot stat '%s': %v\n", src, err)
		return err
	}

	if srcInfo.IsDir() {
		if !opts.Recursive {
			stdio.Errorf("cp: -r not specified; omitting directory '%s'\n", src)
			return os.ErrInvalid
		}
		return copyDir(stdio, src, dest, opts)
	}

	return copyFile(stdio, src, dest, srcInfo, opts)
}

func copyFile(stdio *core.Stdio, src, dest string, srcInfo os.FileInfo, opts *Options) error {
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

		if err := copyPath(stdio, srcPath, destPath, opts); err != nil {
			return err
		}
	}

	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}

	return nil
}
