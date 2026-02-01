// Package cp implements the cp command.
package cp

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rcarmo/busybox-wasm/pkg/core"
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
}

// Run executes the cp command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}

	flagMap := map[byte]*bool{
		'f': &opts.Force,
		'i': &opts.Interactive,
		'p': &opts.Preserve,
		'n': &opts.NoClobber,
		'v': &opts.Verbose,
		'r': &opts.Recursive,
	}
	aliases := map[byte]byte{
		'R': 'r',
	}

	paths, code := core.ParseBoolFlags(stdio, "cp", args, flagMap, aliases)
	if code != core.ExitSuccess {
		return code
	}

	if len(paths) < 2 {
		return core.UsageError(stdio, "cp", "missing file operand")
	}

	dest := paths[len(paths)-1]
	sources := paths[:len(paths)-1]

	// Check if destination is a directory
	destInfo, destErr := fs.Stat(dest)
	destIsDir := destErr == nil && destInfo.IsDir()

	// Multiple sources require directory destination
	if len(sources) > 1 && !destIsDir {
		return core.UsageError(stdio, "cp", "target '"+dest+"' is not a directory")
	}

	exitCode := core.ExitSuccess
	for _, src := range sources {
		target := dest
		if destIsDir {
			target = filepath.Join(dest, filepath.Base(src))
		}

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
		// Interactive mode would prompt here, but for WASM we skip
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
