// Package ls implements the ls command.
package ls

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	sbfs "github.com/rcarmo/busybox-wasm/pkg/core/fs"
	"golang.org/x/term"
)

// Options holds ls command options.
type Options struct {
	All        bool // -a: show hidden files
	AlmostAll  bool // -A: show hidden except . and ..
	Long       bool // -l: long format
	Human      bool // -h: human-readable sizes
	OnePerLine bool // -1: one entry per line
	Recursive  bool // -R: recursive listing
	Reverse    bool // -r: reverse sort order
	SortTime   bool // -t: sort by modification time
	SortSize   bool // -S: sort by size
	NoSort     bool // -f: do not sort
	Classify   bool // -F: append indicator to entries
}

// Run executes the ls command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := Options{}
	paths := []string{}

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
				case 'a':
					opts.All = true
				case 'A':
					opts.AlmostAll = true
				case 'l':
					opts.Long = true
				case 'h':
					opts.Human = true
				case '1':
					opts.OnePerLine = true
				case 'R':
					opts.Recursive = true
				case 'r':
					opts.Reverse = true
				case 't':
					opts.SortTime = true
				case 'S':
					opts.SortSize = true
				case 'f':
					opts.NoSort = true
					opts.All = true
				case 'F':
					opts.Classify = true
				default:
					return core.UsageError(stdio, "ls", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			paths = append(paths, arg)
		}
	}

	if len(paths) == 0 {
		paths = []string{"."}
	}

	if !opts.Long && !opts.OnePerLine {
		opts.OnePerLine = shouldForceOnePerLine(stdio)
	}

	exitCode := core.ExitSuccess
	multiple := len(paths) > 1

	for i, path := range paths {
		if multiple {
			if i > 0 {
				stdio.Println()
			}
			stdio.Printf("%s:\n", path)
		}
		if err := listPath(stdio, path, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func listPath(stdio *core.Stdio, path string, opts *Options) error {
	info, err := sbfs.Stat(path)
	if err != nil {
		stdio.Errorf("ls: cannot access '%s': %v\n", path, err)
		return err
	}

	if !info.IsDir() {
		printEntry(stdio, path, info, opts)
		return nil
	}

	entries, err := sbfs.ReadDir(path)
	if err != nil {
		stdio.Errorf("ls: cannot open directory '%s': %v\n", path, err)
		return err
	}

	// Filter entries
	var filtered []fs.DirEntry
	for _, e := range entries {
		name := e.Name()
		if !opts.All && !opts.AlmostAll && strings.HasPrefix(name, ".") {
			continue
		}
		if opts.AlmostAll && (name == "." || name == "..") {
			continue
		}
		filtered = append(filtered, e)
	}

	// Sort entries
	if !opts.NoSort {
		sortEntries(filtered, opts)
	}

	// Print entries
	for _, e := range filtered {
		info, err := e.Info()
		if err != nil {
			stdio.Errorf("ls: cannot stat '%s': %v\n", e.Name(), err)
			continue
		}
		printEntry(stdio, e.Name(), info, opts)
	}

	// Recursive listing
	if opts.Recursive {
		for _, e := range filtered {
			if e.IsDir() {
				subpath := filepath.Join(path, e.Name())
				stdio.Printf("\n%s:\n", subpath)
				_ = listPath(stdio, subpath, opts)
			}
		}
	}

	return nil
}

func sortEntries(entries []fs.DirEntry, opts *Options) {
	sort.Slice(entries, func(i, j int) bool {
		var less bool

		if opts.SortTime {
			ti, _ := entries[i].Info()
			tj, _ := entries[j].Info()
			less = ti.ModTime().After(tj.ModTime())
		} else if opts.SortSize {
			ti, _ := entries[i].Info()
			tj, _ := entries[j].Info()
			less = ti.Size() > tj.Size()
		} else {
			less = strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		}

		if opts.Reverse {
			return !less
		}
		return less
	})
}

func printEntry(stdio *core.Stdio, name string, info fs.FileInfo, opts *Options) {
	if opts.Long {
		mode := info.Mode()
		size := info.Size()
		modTime := info.ModTime()

		sizeStr := fmt.Sprintf("%d", size)
		if opts.Human {
			sizeStr = humanSize(size)
		}

		stdio.Printf("%s %8s %s %s", mode.String(), sizeStr,
			modTime.Format("Jan _2 15:04"), name)
	} else {
		stdio.Print(name)
	}

	if opts.Classify {
		stdio.Print(classifyChar(info))
	}

	if opts.Long || opts.OnePerLine {
		stdio.Println()
	} else {
		stdio.Print("  ")
	}
}

func classifyChar(info fs.FileInfo) string {
	mode := info.Mode()
	if mode.IsDir() {
		return "/"
	}
	if mode&os.ModeSymlink != 0 {
		return "@"
	}
	if mode&0111 != 0 {
		return "*"
	}
	return ""
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(size)/float64(div), "KMGTPE"[exp])
}

func shouldForceOnePerLine(stdio *core.Stdio) bool {
	if f, ok := stdio.Out.(*os.File); ok {
		return !term.IsTerminal(int(f.Fd()))
	}
	return true
}
