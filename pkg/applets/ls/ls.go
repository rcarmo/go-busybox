// Package ls implements the ls command.
package ls

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
	sbfs "github.com/rcarmo/go-busybox/pkg/core/fs"
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
	DirSlash   bool // -p: append / to directories
	ShowBlocks bool // -s: show allocated blocks
}

// Run executes the ls command with the given arguments.
//
// Supported flags:
//
//	-a    Show all entries including those starting with .
//	-A    Show all except . and ..
//	-l    Use long listing format
//	-h    Human-readable sizes (with -l)
//	-1    One entry per line
//	-R    List directories recursively
//	-r    Reverse sort order
//	-t    Sort by modification time
//	-S    Sort by file size
//	-f    Do not sort (implies -a)
//	-F    Append indicator (*/=>@|) to entries
//	-p    Append / to directories
//	-s    Print allocated size of each file in blocks
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
				case 'p':
					opts.DirSlash = true
				case 's':
					opts.ShowBlocks = true
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
		if err := listPath(stdio, path, path, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func listPath(stdio *core.Stdio, path string, display string, opts *Options) error {
	info, err := sbfs.Stat(path)
	if err != nil {
		stdio.Errorf("ls: cannot access '%s': %v\n", path, err)
		return err
	}

	if !info.IsDir() {
		printEntry(stdio, display, path, info, opts)
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

	// Print total for long format
	if opts.Long {
		var totalBlocks int64
		for _, e := range filtered {
			info, err := e.Info()
			if err != nil {
				continue
			}
			totalBlocks += getBlocks(filepath.Join(path, e.Name()), info)
		}
		stdio.Printf("total %d\n", totalBlocks)
	}

	// Print entries
	for _, e := range filtered {
		info, err := e.Info()
		if err != nil {
			stdio.Errorf("ls: cannot stat '%s': %v\n", e.Name(), err)
			continue
		}
		printEntry(stdio, e.Name(), filepath.Join(path, e.Name()), info, opts)
	}

	// Recursive listing
	if opts.Recursive {
		for _, e := range filtered {
			if e.IsDir() {
				subpath := filepath.Join(path, e.Name())
				stdio.Printf("\n%s:\n", subpath)
				_ = listPath(stdio, subpath, subpath, opts)
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
			less = entries[i].Name() < entries[j].Name()
		}

		if opts.Reverse {
			return !less
		}
		return less
	})
}

func printEntry(stdio *core.Stdio, name string, path string, info fs.FileInfo, opts *Options) {
	if opts.ShowBlocks {
		blocks := getBlocks(path, info)
		stdio.Printf("%6d ", blocks)
	}
	if opts.Long {
		mode := info.Mode()
		size := info.Size()
		modTime := info.ModTime()
		displayName := name
		if mode&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err == nil {
				displayName = fmt.Sprintf("%s -> %s", name, target)
			}
		}

		sizeStr := fmt.Sprintf("%d", size)
		if opts.Human {
			sizeStr = humanSize(size)
		}
		timeStr := formatTime(modTime)

		// Get link count, owner, group from syscall
		nlink := uint64(1)
		owner := "?"
		group := "?"
		if sys := info.Sys(); sys != nil {
			if stat, ok := sys.(*syscall.Stat_t); ok {
				nlink = stat.Nlink
				if u, err := user.LookupId(fmt.Sprintf("%d", stat.Uid)); err == nil {
					owner = u.Username
				} else {
					owner = fmt.Sprintf("%d", stat.Uid)
				}
				if g, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid)); err == nil {
					group = g.Name
				} else {
					group = fmt.Sprintf("%d", stat.Gid)
				}
			}
		}

		stdio.Printf("%s %2d %-8s %-8s %8s %s %s", mode.String(), nlink, owner, group, sizeStr, timeStr, displayName)
	} else {
		stdio.Print(name)
	}

	if opts.Classify {
		stdio.Print(classifyChar(info))
	} else if opts.DirSlash && info.IsDir() {
		stdio.Print("/")
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

func formatTime(t time.Time) string {
	now := time.Now()
	sixMonthsAgo := now.AddDate(0, -6, 0)
	if t.Before(sixMonthsAgo) || t.After(now.AddDate(0, 0, 1)) {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

func getBlocks(path string, info fs.FileInfo) int64 {
	if sys := info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			// st_blocks is in 512-byte units; convert to 1024-byte
			return stat.Blocks / 2
		}
	}
	// Fallback: estimate from size (round up to 4K blocks)
	return (info.Size() + 4095) / 4096 * 4
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
