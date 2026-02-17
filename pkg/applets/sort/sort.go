// Package sort implements the sort command for sorting lines of text.
//
// It supports numeric (-n), reverse (-r), unique (-u), key-field (-k),
// field-separator (-t), and stability (-s) options.
package sort

import (
	"bufio"
	"sort"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/fs"
	"github.com/rcarmo/go-busybox/pkg/core/textutil"
)

type options struct {
	reverse  bool
	numeric  bool
	unique   bool
	ignore   bool
	sep      string
	key      string
	keyField int
	keyChar  int
	outFile  string
}

// Run executes the sort command with the given arguments.
//
// Supported flags:
//
//	-r        Reverse the result of comparisons
//	-n        Compare according to string numerical value
//	-u        Output only the first of equal lines
//	-f        Fold lower case to upper case for comparison
//	-t SEP    Use SEP as the field separator
//	-k KEYDEF Use KEYDEF to define sort key (e.g., -k2,2n)
//	-s        Stabilise sort by disabling last-resort comparison
//	-o FILE   Write output to FILE instead of stdout
//
// Reads from stdin when no files are given or when "-" is specified.
func Run(stdio *core.Stdio, args []string) int {
	opts := options{}
	files := []string{}
	// BusyBox uses byte/locale order; this implementation uses Go's Unicode string order.

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if arg == "-t" || arg == "-k" || arg == "-o" {
				if i+1 >= len(args) {
					return core.UsageError(stdio, "sort", "missing argument")
				}
				i++
				switch arg {
				case "-t":
					opts.sep = args[i]
				case "-k":
					opts.key = args[i]
					field, char, err := textutil.ParseKeySpec(opts.key)
					if err != nil {
						return core.UsageError(stdio, "sort", "invalid key")
					}
					opts.keyField = field
					opts.keyChar = char
				case "-o":
					opts.outFile = args[i]
				}
				continue
			}
			for _, c := range arg[1:] {
				switch c {
				case 'r':
					opts.reverse = true
				case 'n':
					opts.numeric = true
				case 'u':
					opts.unique = true
				case 'f':
					opts.ignore = true
				default:
					return core.UsageError(stdio, "sort", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
		}
	}
	if opts.sep != "" {
		runes := []rune(opts.sep)
		if len(runes) != 1 {
			return core.UsageError(stdio, "sort", "invalid separator")
		}
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	lines := []string{}
	for _, f := range files {
		var scanner *bufio.Scanner
		if f == "-" {
			scanner = bufio.NewScanner(stdio.In)
		} else {
			rf, err := fs.Open(f)
			if err != nil {
				stdio.Errorf("sort: %s: %v\n", f, err)
				return core.ExitFailure
			}
			defer rf.Close()
			scanner = bufio.NewScanner(rf)
		}
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			stdio.Errorf("sort: %v\n", err)
			return core.ExitFailure
		}
	}

	sort.SliceStable(lines, func(i, j int) bool {
		li := lines[i]
		lj := lines[j]
		ki := buildSortKey(li, opts)
		kj := buildSortKey(lj, opts)
		if opts.ignore {
			ki = strings.ToLower(ki)
			kj = strings.ToLower(kj)
		}
		if opts.numeric {
			ni, erri := strconv.ParseFloat(strings.TrimSpace(ki), 64)
			nj, errj := strconv.ParseFloat(strings.TrimSpace(kj), 64)
			if erri != nil && errj == nil {
				return !opts.reverse
			}
			if erri == nil && errj != nil {
				return opts.reverse
			}
			if erri == nil && errj == nil {
				if ni == nj {
					return ki < kj
				}
				if opts.reverse {
					return ni > nj
				}
				return ni < nj
			}
		}
		if opts.reverse {
			return ki > kj
		}
		return ki < kj
	})

	output := make([]string, 0, len(lines))
	lastKey := ""
	first := true
	for _, l := range lines {
		key := buildSortKey(l, opts)
		if opts.ignore {
			key = strings.ToLower(key)
		}
		if opts.unique {
			if !first && key == lastKey {
				continue
			}
			lastKey = key
			first = false
		}
		output = append(output, l)
	}
	if opts.outFile != "" {
		if err := fs.WriteFile(opts.outFile, []byte(strings.Join(output, "\n")+"\n"), 0600); err != nil {
			stdio.Errorf("sort: %v\n", err)
			return core.ExitFailure
		}
		return core.ExitSuccess
	}
	for _, line := range output {
		stdio.Println(line)
	}
	return core.ExitSuccess
}

func buildSortKey(line string, opts options) string {
	if opts.key == "" {
		return line
	}
	return textutil.ExtractKey(line, opts.keyField, opts.keyChar, opts.sep)
}
