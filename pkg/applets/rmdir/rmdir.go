// Package rmdir implements the rmdir command.
package rmdir

import (
	"os"
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Run executes the rmdir command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	parents := false
	verbose := false
	var dirs []string

	flagMap := map[byte]*bool{
		'p': &parents,
		'v': &verbose,
	}

	paths, code := core.ParseBoolFlags(stdio, "rmdir", args, flagMap, nil)
	if code != core.ExitSuccess {
		return code
	}
	dirs = append(dirs, paths...)

	if len(dirs) == 0 {
		return core.UsageError(stdio, "rmdir", "missing operand")
	}

	exitCode := core.ExitSuccess
	for _, dir := range dirs {
		if err := removeDir(stdio, dir, parents, verbose); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func removeDir(stdio *core.Stdio, dir string, parents bool, verbose bool) error {
	if err := fs.Remove(dir); err != nil {
		if os.IsNotExist(err) {
			stdio.Errorf("rmdir: failed to remove '%s': No such file or directory\n", dir)
			return err
		}
		stdio.Errorf("rmdir: failed to remove '%s': %v\n", dir, err)
		return err
	}

	if verbose {
		stdio.Printf("rmdir: removed directory '%s'\n", dir)
	}

	if parents {
		parent := filepath.Dir(dir)
		if parent != "." && parent != "/" {
			_ = removeDir(stdio, parent, parents, verbose)
		}
	}

	return nil
}
