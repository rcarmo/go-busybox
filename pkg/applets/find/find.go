// Package find implements a minimal subset of find.
package find

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
)

type actionType int

const (
	actionPrint actionType = iota
	actionPrint0
	actionDelete
	actionPrune
)

type options struct {
	followSymlinks  bool
	minDepth        int
	maxDepth        int
	namePattern     string
	nameInsensitive bool
	pathPattern     string
	pathInsensitive bool
	typeFilter      rune
	print0          bool
	prune           bool
	sizeFilter      *sizeFilter
	mtimeFilter     *timeFilter
	atimeFilter     *timeFilter
	ctimeFilter     *timeFilter
	actions         []actionType
}

type sizeFilter struct {
	op   byte
	size int64
	unit int64
}

type timeFilter struct {
	op   byte
	days int
}

// Run executes the find command with the given arguments.
func Run(stdio *core.Stdio, args []string) int {
	opts := options{
		minDepth: 0,
		maxDepth: -1,
	}
	paths := []string{}
	i := 0

	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		paths = append(paths, args[i])
		i++
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	for i < len(args) {
		arg := args[i]
		switch arg {
		case "-L", "-follow":
			opts.followSymlinks = true
		case "-H":
			opts.followSymlinks = true
		case "-xdev", "-mount":
			// Don't cross filesystem boundaries - accepted but not enforced
			// in this minimal implementation
		case "-name":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-name'")
			}
			opts.namePattern = args[i]
		case "-iname":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-iname'")
			}
			opts.namePattern = args[i]
			opts.nameInsensitive = true
		case "-path":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-path'")
			}
			opts.pathPattern = args[i]
		case "-ipath":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-ipath'")
			}
			opts.pathPattern = args[i]
			opts.pathInsensitive = true
		case "-type":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-type'")
			}
			if len(args[i]) != 1 {
				return core.UsageError(stdio, "find", "invalid -type")
			}
			opts.typeFilter = rune(args[i][0])
		case "-mindepth":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-mindepth'")
			}
			val, err := parseInt(args[i])
			if err != nil || val < 0 {
				return core.UsageError(stdio, "find", "invalid -mindepth")
			}
			opts.minDepth = val
		case "-maxdepth":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-maxdepth'")
			}
			val, err := parseInt(args[i])
			if err != nil || val < 0 {
				return core.UsageError(stdio, "find", "invalid -maxdepth")
			}
			opts.maxDepth = val
		case "-print0":
			opts.print0 = true
			opts.actions = append(opts.actions, actionPrint0)
		case "-print":
			opts.actions = append(opts.actions, actionPrint)
		case "-prune":
			opts.prune = true
			opts.actions = append(opts.actions, actionPrune)
		case "-delete":
			opts.actions = append(opts.actions, actionDelete)
		case "-size":
			i++
			if i >= len(args) {
				return core.UsageError(stdio, "find", "missing argument to '-size'")
			}
			filter, err := parseSize(args[i])
			if err != nil {
				return core.UsageError(stdio, "find", "invalid -size")
			}
			opts.sizeFilter = filter
		case "-mtime":
			i++
			filter, err := parseTimeFilter(args, i, "mtime")
			if err != nil {
				return core.UsageError(stdio, "find", err.Error())
			}
			opts.mtimeFilter = filter
		case "-atime":
			i++
			filter, err := parseTimeFilter(args, i, "atime")
			if err != nil {
				return core.UsageError(stdio, "find", err.Error())
			}
			opts.atimeFilter = filter
		case "-ctime":
			i++
			filter, err := parseTimeFilter(args, i, "ctime")
			if err != nil {
				return core.UsageError(stdio, "find", err.Error())
			}
			opts.ctimeFilter = filter
		default:
			if strings.HasPrefix(arg, "-") {
				return core.UsageError(stdio, "find", "unknown predicate: "+arg)
			}
		}
		i++
	}

	if len(opts.actions) == 0 {
		if opts.print0 {
			opts.actions = append(opts.actions, actionPrint0)
		} else {
			opts.actions = append(opts.actions, actionPrint)
		}
	}

	exitCode := core.ExitSuccess
	for _, root := range paths {
		if err := walkPath(stdio, root, &opts); err != nil {
			exitCode = core.ExitFailure
		}
	}

	return exitCode
}

func walkPath(stdio *core.Stdio, root string, opts *options) error {
	info, err := fs.Stat(root)
	if err != nil {
		stdio.Errorf("find: '%s': %v\n", root, err)
		return err
	}
	return walkRecursive(stdio, root, info, opts, 0)
}

func walkRecursive(stdio *core.Stdio, path string, info os.FileInfo, opts *options, depth int) error {
	matched := match(path, info, opts)
	if depth >= opts.minDepth && matched {
		if err := applyActions(stdio, path, info, opts, matched); err != nil {
			return err
		}
	}

	if !info.IsDir() {
		return nil
	}
	if opts.maxDepth >= 0 && depth >= opts.maxDepth {
		return nil
	}
	if matched && opts.prune {
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
		if opts.followSymlinks {
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

func match(path string, info os.FileInfo, opts *options) bool {
	if opts.namePattern != "" {
		name := filepath.Base(path)
		if opts.nameInsensitive {
			if !matchPattern(strings.ToLower(opts.namePattern), strings.ToLower(name)) {
				return false
			}
		} else if !matchPattern(opts.namePattern, name) {
			return false
		}
	}
	if opts.pathPattern != "" {
		target := filepath.ToSlash(path)
		pattern := opts.pathPattern
		if opts.pathInsensitive {
			pattern = strings.ToLower(pattern)
			target = strings.ToLower(target)
		}
		if matchPattern(pattern, target) {
			// matched
		} else if !strings.HasPrefix(target, "./") && matchPattern(pattern, "./"+target) {
			// matched with ./ prefix
		} else {
			return false
		}
	}
	if opts.typeFilter != 0 {
		switch opts.typeFilter {
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
	if opts.sizeFilter != nil {
		if !matchSize(info, opts.sizeFilter) {
			return false
		}
	}
	if opts.mtimeFilter != nil {
		if !matchTime(info.ModTime(), opts.mtimeFilter) {
			return false
		}
	}
	if opts.atimeFilter != nil {
		if !matchTime(info.ModTime(), opts.atimeFilter) {
			return false
		}
	}
	if opts.ctimeFilter != nil {
		if !matchTime(info.ModTime(), opts.ctimeFilter) {
			return false
		}
	}
	return true
}

func applyActions(stdio *core.Stdio, path string, info os.FileInfo, opts *options, matched bool) error {
	for _, action := range opts.actions {
		switch action {
		case actionPrint:
			stdio.Println(path)
		case actionPrint0:
			stdio.Print(path, "\x00")
		case actionDelete:
			if info.IsDir() {
				if err := fs.Remove(path); err != nil {
					stdio.Errorf("find: '%s': %v\n", path, err)
					return err
				}
			} else {
				if err := fs.Remove(path); err != nil {
					stdio.Errorf("find: '%s': %v\n", path, err)
					return err
				}
			}
		case actionPrune:
			opts.prune = true
		}
	}
	return nil
}

func matchPattern(pattern string, name string) bool {
	if matched, err := filepath.Match(pattern, name); err == nil && matched {
		return true
	}
	for i := 0; i < len(name); i++ {
		if matched, err := filepath.Match(pattern, name[i:]); err == nil && matched {
			return true
		}
	}
	return false
}

func parseSize(val string) (*sizeFilter, error) {
	if val == "" {
		return nil, os.ErrInvalid
	}
	filter := &sizeFilter{op: 0, unit: 512}
	if val[0] == '+' || val[0] == '-' {
		filter.op = val[0]
		val = val[1:]
	}
	if val == "" {
		return nil, os.ErrInvalid
	}
	unit := val[len(val)-1]
	if unit == 'c' || unit == 'k' || unit == 'b' {
		switch unit {
		case 'c':
			filter.unit = 1
		case 'k':
			filter.unit = 1024
		case 'b':
			filter.unit = 512
		}
		val = val[:len(val)-1]
	}
	size, err := parseInt(val)
	if err != nil {
		return nil, err
	}
	filter.size = int64(size)
	return filter, nil
}

func matchSize(info os.FileInfo, filter *sizeFilter) bool {
	size := info.Size() / filter.unit
	switch filter.op {
	case '+':
		return size > filter.size
	case '-':
		return size < filter.size
	default:
		return size == filter.size
	}
}

func parseTimeFilter(args []string, i int, label string) (*timeFilter, error) {
	if i >= len(args) {
		return nil, os.ErrInvalid
	}
	val := args[i]
	if val == "" {
		return nil, os.ErrInvalid
	}
	filter := &timeFilter{op: 0}
	if val[0] == '+' || val[0] == '-' {
		filter.op = val[0]
		val = val[1:]
	}
	n, err := parseInt(val)
	if err != nil {
		return nil, os.ErrInvalid
	}
	filter.days = n
	return filter, nil
}

func matchTime(t time.Time, filter *timeFilter) bool {
	ageDays := int(time.Since(t).Hours() / 24)
	switch filter.op {
	case '+':
		return ageDays > filter.days
	case '-':
		return ageDays < filter.days
	default:
		return ageDays == filter.days
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
