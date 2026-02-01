// Package mv implements the mv command.
package mv

import (
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Options holds mv command options.
type Options struct {
	Force       bool // -f: force overwrite
	Interactive bool // -i: prompt before overwrite
	NoClobber   bool // -n: do not overwrite existing files
	Verbose     bool // -v: verbose output
}

// Run executes the mv command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}

	flagMap := map[byte]*bool{
		'f': &opts.Force,
		'i': &opts.Interactive,
		'n': &opts.NoClobber,
		'v': &opts.Verbose,
	}

	paths, code := core.ParseBoolFlags(stdio, "mv", args, flagMap)
	if code != core.ExitSuccess {
		return code
	}

	if len(paths) < 2 {
		return core.UsageError(stdio, "mv", "missing file operand")
	}

	dest := paths[len(paths)-1]
	sources := paths[:len(paths)-1]

	// Check if destination is a directory
	destInfo, destErr := fs.Stat(dest)
	destIsDir := destErr == nil && destInfo.IsDir()

	// Multiple sources require directory destination
	if len(sources) > 1 && !destIsDir {
		return core.UsageError(stdio, "mv", "target '"+dest+"' is not a directory")
	}

	exitCode := core.ExitSuccess
	for _, src := range sources {
		target := dest
		if destIsDir {
			target = filepath.Join(dest, filepath.Base(src))
		}

		if err := movePath(stdio, src, target, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func movePath(stdio *core.Stdio, src, dest string, opts *Options) error {
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
		// Force mode removes destination first
		if opts.Force {
			_ = fs.Remove(dest)
		}
	}

	// Try rename first (works for same filesystem)
	if err := fs.Rename(src, dest); err != nil {
		stdio.Errorf("mv: cannot move '%s' to '%s': %v\n", src, dest, err)
		return err
	}

	if opts.Verbose {
		stdio.Printf("'%s' -> '%s'\n", src, dest)
	}

	return nil
}
