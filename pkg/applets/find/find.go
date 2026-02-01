// Package find implements a minimal subset of find.
package find

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

// Options holds find command options.
type Options struct {
	FollowSymlinks bool
	MinDepth       int
	MaxDepth       int
	NamePattern    string
	TypeFilter     rune
	Print0         bool
}

// Run executes the find command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{
		MinDepth: 0,
		MaxDepth: -1,
	}

	paths := []string{}
	i := 0

	// Parse paths first (until an option is found)
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		paths = append(paths, args[i])
		i++
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Parse options
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "-L", "-follow":
			opts.FollowSymlinks = true
		case "-H":
			// Busybox treats -H as follow command line symlinks only.
			// We treat it the same as -L for now.
			opts.FollowSymlinks = true
		case "-name":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-name'")
			}
			opts.NamePattern = args[i]
		case "-type":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-type'")
			}
			if len(args[i]) != 1 {
				return core.UsageError(stdio, "find", "invalid -type")
			}
			opts.TypeFilter = rune(args[i][0])
		case "-mindepth":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-mindepth'")
			}
			val, err := parseInt(args[i])
			if err != nil || val < 0 {
				return core.UsageError(stdio, "find", "invalid -mindepth")
			}
			opts.MinDepth = val
		case "-maxdepth":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-maxdepth'")
			}
			val, err := parseInt(args[i])
			if err != nil || val < 0 {
				return core.UsageError(stdio, "find", "invalid -maxdepth")
			}
			opts.MaxDepth = val
		case "-print0":
			opts.Print0 = true
		case "-print":
			// default, ignore
		default:
			if strings.HasPrefix(arg, "-") {
				return core.UsageError(stdio, "find", "unknown predicate: "+arg)
			}
		}
		i++
	}

	exitCode := core.ExitSuccess
	for _, root := range paths {
		if err := walkPath(stdio, root, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func walkPath(stdio *core.Stdio, root string, opts *Options) error {
	info, err := fs.Stat(root)
	if err != nil {
		stdio.Errorf("find: '%s': %v\n", root, err)
		return err
	}

	return walkRecursive(stdio, root, info, opts, 0)
}

func walkRecursive(stdio *core.Stdio, path string, info os.FileInfo, opts *Options, depth int) error {
	if depth >= opts.MinDepth {
		if match(path, info, opts) {
			printPath(stdio, path, opts)
		}
	}

	if !info.IsDir() {
		return nil
	}

	if opts.MaxDepth >= 0 && depth >= opts.MaxDepth {
		return nil
	}

	entries, err := fs.ReadDir(path)
	if err != nil {
		stdio.Errorf("find: '%s': %v\n", path, err)
		return err
	}

	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		var childInfo os.FileInfo
		if opts.FollowSymlinks {
			childInfo, err = fs.Stat(childPath)
		} else {
			childInfo, err = fs.Lstat(childPath)
		}
		if err != nil {
			stdio.Errorf("find: '%s': %v\n", childPath, err)
			continue
		}
		if err := walkRecursive(stdio, childPath, childInfo, opts, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func match(path string, info os.FileInfo, opts *Options) bool {
	if opts.NamePattern != "" {
		name := filepath.Base(path)
		matched, err := filepath.Match(opts.NamePattern, name)
		if err != nil || !matched {
			return false
		}
	}

	if opts.TypeFilter != 0 {
		switch opts.TypeFilter {
		case 'f':
			if !info.Mode().IsRegular() {
				return false
			}
		case 'd':
			if !info.IsDir() {
				return false
			}
		case 'l':
			if info.Mode()&os.ModeSymlink == 0 {
				return false
			}
		default:
			return false
		}
	}

	return true
}

func printPath(stdio *core.Stdio, path string, opts *Options) {
	if opts.Print0 {
		stdio.Print(path, "\x00")
	} else {
		stdio.Println(path)
	}
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
