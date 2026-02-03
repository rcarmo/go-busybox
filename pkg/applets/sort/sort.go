package sort

import (
	"bufio"
	"sort"
	"strconv"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

type options struct {
	reverse bool
	numeric bool
	unique  bool
}

func Run(stdio *core.Stdio, args []string) int {
	opts := options{}
	files := []string{}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'r':
					opts.reverse = true
				case 'n':
					opts.numeric = true
				case 'u':
					opts.unique = true
				default:
					return core.UsageError(stdio, "sort", "invalid option -- '"+string(c)+"'")
				}
			}
		} else {
			files = append(files, arg)
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
		if opts.numeric {
			ni, erri := strconv.ParseFloat(strings.TrimSpace(li), 64)
			nj, errj := strconv.ParseFloat(strings.TrimSpace(lj), 64)
			if erri != nil && errj == nil {
				return !opts.reverse
			}
			if erri == nil && errj != nil {
				return opts.reverse
			}
			if erri == nil && errj == nil {
				if ni == nj {
					return li < lj
				}
				if opts.reverse {
					return ni > nj
				}
				return ni < nj
			}
		}
		if opts.reverse {
			return li > lj
		}
		return li < lj
	})

	last := ""
	first := true
	for _, l := range lines {
		if opts.unique {
			if !first && l == last {
				continue
			}
			last = l
			first = false
		}
		stdio.Println(l)
	}
	return core.ExitSuccess
}
